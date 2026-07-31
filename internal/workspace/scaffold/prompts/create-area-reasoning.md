# Create Area Reasoning

> **Inputs:** technical-handbook.md, requirements.md, sprint-index.md, directly relevant selected evidence reports, docs/*.md, injected selected reasoning template

---

Use this prompt to create optional area-specific reasoning documents for a sprint.

## Required Inputs

Read these files first:

1. Technical handbook: `.ultra/projects/{project}/sprints/{sprint-slug}/technical-handbook.md`
   - Treat the handbook as a high-level map of the selected evidence, cross-cutting patterns, cautions, and open questions.
   - Do not treat the handbook as a substitute for the underlying reports when they directly influence the selected area.
2. Sprint requirements: `.ultra/projects/{project}/sprints/{sprint-slug}/requirements.md`
3. Sprint index: `.ultra/projects/{project}/sprints/{sprint-slug}/sprint-index.md`
   - Use its selected-evidence entries to determine which reports are permitted inputs.
4. Selected evidence reports listed in the runtime manifest and sprint index
   - Identify the reports that directly influence the selected area and read those reports in depth.
   - Do not read unrelated or unselected reports merely to broaden the analysis.
5. Project docs: all markdown files in `.ultra/projects/{project}/docs/*.md`
6. Injected selected reasoning template section in this prompt

## Output

For each area selected by sprint-index, write to:
`.ultra/projects/{project}/sprints/{sprint-slug}/reasoning/<area>.md`

Only create files for areas explicitly selected in sprint-index. Do NOT create area reasoning documents for ceremony.

## Instructions

1. The flow has already determined which area files to create. Do not infer extra areas.
2. Use the technical handbook for orientation and to understand the high-level evidence landscape.
3. For the current area, identify which selected evidence reports materially affect its decisions, constraints, trade-offs, failure modes, or risks.
4. Read those directly relevant reports in depth before reaching conclusions. Follow their evidence pointers or code references when necessary to understand a finding, but remain within the context selected by sprint-index.
5. Do not force every selected report into every area document. Use reports according to their actual relevance, and never read or cite an unselected report.
6. Read `requirements.md` and all project docs in `docs/` for sprint-specific scope and constraints.
7. For each selected area:
   - Use the injected selected reasoning template section as source material to reason through, not as the literal output structure
   - At the very top of the file, add an `> **Inputs Used:**` line listing the exact files used for that document, including every underlying report read directly
   - Include these exact required `##` sections with concrete content:
     - `## Area Decisions`
     - `## Trade-Offs`
     - `## Evidence`
     - `## Risks`
   - Ground area-specific decisions primarily in the directly relevant selected reports, using the handbook for high-level synthesis and cross-cutting context
   - Cite the exact report paths supporting material decisions and distinguish report findings from your own inference
   - Record the key conclusion and evidence basis
   - Note any open questions or risks
8. Do not duplicate content from technical-handbook — use it as an overview, then synthesize deeper report evidence into area-specific conclusions.
9. Ensure each area reasoning document is self-contained and can be understood without reading other area documents.

The UltraPlan runtime manifest may describe the permitted inputs as "selected context from sprint-index.md and technical-handbook.md." Interpret selected context to include the underlying evidence reports explicitly selected by sprint-index and listed in the manifest. The handbook is an overview of that evidence, not the sole evidence source.

## Skip Criteria

Skip creating area reasoning if:

- No areas are selected in sprint-index
- Area reasoning files already exist and are complete
- Contains placeholders

## Quality Bar

Each area reasoning document must:

- Have a clear area name and scope
- Use the technical handbook as high-level orientation
- Cite every selected report that materially influences the area, without padding the document with irrelevant reports
- Show evidence of deeper report analysis where the area depends on report findings
- Make final area-specific decisions (no more "TBD")
- Record rejected alternatives
- Note risks, assumptions, and open questions
- Be referenced in sprint-reasoning.md
