# Create Sprint Requirements

> **Inputs:** project-index.md, roadmap.md, docs/*.md, requirements template, prior sprint reviews

---

Use this prompt to create the sprint requirements document for a new sprint.

## Required Inputs

Read these files first:

1. Project index: `.ultra/projects/{project}/project-index.md`
2. Project roadmap: `.ultra/projects/{project}/roadmap.md`
3. Project docs: all markdown files in `.ultra/projects/{project}/docs/*.md`
4. Requirements template: `.ultra/system/templates/requirements.md`
5. Prior sprint decisions (if any): `.ultra/projects/{project}/sprints/*/review.md`

## Output

Write to: `.ultra/projects/{project}/sprints/{sprint-slug}/requirements.md`

## Instructions

1. Read the project index and roadmap to understand the overall project scope and sprint sequence.
2. Read all project docs in `docs/` for requirements context.
3. If this is not the first sprint, read prior sprint review(s) for carry-forward decisions.
4. Fill the requirements template with:
   - **Sprint Goal**: one clear sentence describing what must be achieved.
   - **Required Outputs**: enumerate every deliverable file with its path and a one-line description.
   - **Acceptance Criteria**: checkable criteria that prove the sprint succeeded.
   - **Non-Goals**: explicitly what is NOT included.
   - **Constraints**: architectural, technical, or process constraints.
   - **Dependencies**: prior sprints or external inputs this sprint needs.
   - **Review Expectations**: what will be checked at review and how.

## Quality Bar

- The sprint goal must be achievable in one sprint.
- All required outputs must be specific (file paths, not vague descriptions).
- Acceptance criteria must be objectively verifiable.
- Non-goals must be specific enough to prevent scope creep.
- Constraints must be binding (not aspirational).

## Skip Criteria

Skip creating requirements if:

- `requirements.md` already exists and is complete
- Required inputs (roadmap, project-index) are missing
