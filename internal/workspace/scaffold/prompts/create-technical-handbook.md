# Create Technical Handbook

> **Inputs:** `sprint-index.md`, `project-index.md`, `project-index.md`, `requirements.md`, `docs/*.md`, reports selected in `sprint-index.md` → "Selected Evidence Reports" section, technical-handbook template

---

Use this prompt to create the technical handbook for a sprint.

## Required Inputs

Read these files first:

1. Sprint index: `.ultra/projects/{project}/sprints/{sprint-slug}/sprint-index.md` — contains the **Selected Evidence Reports** section that tells you which reports to read
2. Technical handbook template: `.ultra/system/templates/technical-handbook.md`

Then read the evidence reports listed in the sprint index's "Selected Evidence Reports" section. The project index's "Available Evidence Reports" table is the authoritative source for those report paths.

Do NOT read all evidence reports indiscriminately. Only read the ones the sprint index selects.

## Output

Write to: `.ultra/projects/{project}/sprints/{sprint-slug}/technical-handbook.md`

Use the technical-handbook template. Fill every section. The handbook distills selected studies and reports into sprint-relevant patterns, trade-offs, cautions, and questions. It does NOT decide implementation.

## Instructions

1. Read the sprint index first to find which evidence reports to read.
2. Read those evidence reports directly. Evidence reports cite specific source files and line numbers from real study repos (e.g., `mitchellh-cli/cli.go:70`, `go-task/cmd/task/task.go:23-46`). These are your evidence base.
3. Read `requirements.md` for sprint scope context.
4. Extract from the evidence reports:
   - Relevant patterns that apply to this sprint's scope
   - Trade-offs observed across sources
   - Warnings or anti-patterns to avoid
   - Open questions that sprint reasoning must resolve
   - Design pressures visible in the study evidence
5. At the top of `technical-handbook.md`, write `> **Inputs Used:**` listing the exact files used.
6. Do not use contracts, PRD, TRD, or other governance documents as handbook evidence. They may provide scope context only.
7. Do not hardcode patterns — ground them in the evidence from the reports.
8. Do not make final implementation decisions — defer those to sprint reasoning.
9. Cite specific source files and line numbers from the reports (e.g., `mitchellh-cli/cli.go:70`).

## Skip Criteria

Skip creating technical-handbook if:

- `technical-handbook.md` already exists and is complete
- Sprint index is missing or has no "Selected Evidence Reports" section
- Selected evidence reports cannot be located

## Quality Bar

The technical handbook must:

- Cite specific evidence reports and their findings
- Identify at least 3 relevant patterns with evidence basis (file paths + line numbers from evidence reports)
- Document at least 2 trade-offs with benefit/cost analysis
- Include at least 2 warnings or anti-patterns
- List open questions for sprint reasoning
- Point to specific evidence (file paths, sections) to inspect
