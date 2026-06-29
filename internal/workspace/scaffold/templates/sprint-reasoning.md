# Sprint Reasoning: [Sprint Name]

> Project: `[project-slug]`
> Sprint: `[sprint-slug]`
> Output: `.ultra/projects/[project-slug]/sprints/[sprint-slug]/reasoning.md`
> **Inputs Used:** `.ultra/projects/[project-slug]/project-index.md`, `.ultra/projects/[project-slug]/sprints/[sprint-slug]/requirements.md`, `.ultra/projects/[project-slug]/docs/*.md`, `.ultra/projects/[project-slug]/sprints/[sprint-slug]/sprint-index.md`, `.ultra/projects/[project-slug]/sprints/[sprint-slug]/technical-handbook.md`, `reasoning/*.md` (if any)

This document decides. It synthesizes selected context, handbook evidence, area-specific reasoning, and contracts into final sprint decisions.

It does not replace `sprint-index.md`, `technical-handbook.md`, or `reasoning/*.md`.

## Sprint Purpose

- **Goal:** `[what this sprint delivers]`
- **Non-Goals:** `[what this sprint will not do]`
- **Depends On:** `[prior sprint, decision, or none]`

## Selected Context And Pre-Reasoning Artifacts

| Artifact | Path | How It Was Used |
| --- | --- | --- |
| Sprint Index | `sprint-index.md` | `[selected context]` |
| Technical Handbook | `technical-handbook.md` | `[evidence summary]` |
| Prior Decision | `[path]` | `[constraint carried forward]` |

## Area-Specific Reasoning Inputs

Summarize the actual area-specific reasoning documents created for this sprint. Do not hardcode required reasoning documents here; use only the artifacts that actually exist for this sprint.

| Area | Reasoning Document | Key Conclusion | Evidence Basis | Impact On Final Decision |
| --- | --- | --- | --- | --- |
| `[area selected in sprint-index.md]` | `reasoning/[artifact].md` | `[summary conclusion]` | `[evidence/trade-offs used]` | `[how it shapes final decisions]` |
| Example: Architecture | `reasoning/architecture.md` | `[summary conclusion]` | `[evidence/trade-offs used]` | `[how it shapes final decisions]` |

If no area-specific reasoning is selected by `sprint-index.md`, write `None selected - [reason]`.

## Sprint Technical Handbook Summary

- **Relevant Patterns:** `[patterns used as evidence]`
- **Important Trade-Offs:** `[trade-offs that matter]`
- **Warnings / Anti-Patterns:** `[warnings considered]`
- **Evidence Confidence:** `[high/medium/low and why]`

## Contracts Applied

| Contract / Requirement ID | Constraint | Decision Impact | Expected Evidence |
| --- | --- | --- | --- |
| `[contract or ID]` | `[constraint]` | `[how it shapes work]` | `[evidence]` |

## Repos Studied / Source Evidence Used

List the concrete repositories, source reports, or cited examples that materially shaped this sprint's decisions. These should come from `technical-handbook.md` and the selected evidence reports. If a sprint is primarily documentation, housekeeping, or otherwise not source-evidence-driven, explicitly state that no study repositories were relevant and justify why.

| Source / Repo / Report | Concrete Reference | Relevant Finding | Why It Matters For This Sprint | Used In Decision(s) |
| --- | --- | --- | --- | --- |
| `[repo or report]` | `[report path, section, source file:line, or cited repo path]` | `[finding]` | `[why relevant]` | `[decision name(s) or none]` |

## Trade-Off And Debt Analysis

Analyze the consequences of the chosen direction before listing final decisions.

### Accepted Trade-Offs

| Trade-Off | Benefit | Cost / Constraint Accepted | Why Acceptable Now | Revisit Trigger |
| --- | --- | --- | --- | --- |
| `[trade-off]` | `[benefit]` | `[cost]` | `[why acceptable for this sprint]` | `[condition that should reopen it]` |

### Potential Technical Debt

| Debt / Shortcut | Why It Might Accrue | Current Mitigation | Owner / Follow-Up |
| --- | --- | --- | --- |
| `[debt]` | `[risk mechanism]` | `[mitigation]` | `[future sprint / decision log]` |

### Future Considerations

| Consideration | Deferred Until | Reason Deferred | What Should Be Preserved Now |
| --- | --- | --- | --- |
| `[future concern]` | `[sprint / milestone / trigger]` | `[why not now]` | `[seam, invariant, or documentation to preserve]` |

## Final Decisions

### Decision 1: [Decision Name]

- **Decision:** `[what will be done]`
- **Rationale:** `[why this is the right choice for this sprint]`
- **Study / Source Grounding:** `[technical-handbook/report/repo/source references that support this decision, or "No study sources were relevant because ..."]`
- **Trade-Offs Accepted:** `[benefits/costs accepted for this decision]`
- **Technical Debt / Future Impact:** `[debt introduced, avoided, or deferred; future considerations]`
- **Alternatives Rejected:** `[alternatives and why rejected]`
- **Contracts Satisfied:** `[contracts / requirement IDs]`
- **Evidence Required:** `[tests, runtime evidence, review checks]`

### Decision 2: [Decision Name]

- **Decision:** `[what will be done]`
- **Rationale:** `[why this is the right choice for this sprint]`
- **Study / Source Grounding:** `[technical-handbook/report/repo/source references that support this decision, or "No study sources were relevant because ..."]`
- **Trade-Offs Accepted:** `[benefits/costs accepted for this decision]`
- **Technical Debt / Future Impact:** `[debt introduced, avoided, or deferred; future considerations]`
- **Alternatives Rejected:** `[alternatives and why rejected]`
- **Contracts Satisfied:** `[contracts / requirement IDs]`
- **Evidence Required:** `[tests, runtime evidence, review checks]`

## Expected Evidence

| Evidence Type | Required Evidence | Source / Command / Review Check |
| --- | --- | --- |
| Tests | `[required test evidence]` | `[command or path]` |
| Runtime | `[logs/events/diagnostics]` | `[path or method]` |
| Review | `[review confirmation]` | `[protocol/check]` |
| Documentation | `[docs that must change]` | `[path]` |

## Assumptions And Risks

| Item | Type | Impact | Mitigation / Follow-Up |
| --- | --- | --- | --- |
| `[assumption or risk]` | `[assumption/risk]` | `[impact]` | `[mitigation]` |

## Implementation Constraints

- `[constraint the plan and implementation must respect]`
- `[constraint the plan and implementation must respect]`

## Plan Handoff

`plan.md` must execute these decisions. It must not invent architecture, scope, or decisions beyond this document.

The plan must carry forward:

- final decisions
- applicable contracts and requirement IDs
- expected evidence
- risks and assumptions
- required review protocols

## Phase Exit Criteria

- [ ] Selected context was read and used.
- [ ] Area-specific reasoning documents were completed where applicable or explicitly marked not applicable.
- [ ] Area-specific reasoning conclusions are reflected or explicitly overridden in final decisions.
- [ ] Contracts are explicitly mapped to decisions and expected evidence.
- [ ] Final decisions are clear enough for `plan.md` to execute.
- [ ] Expected evidence is specific and reviewable.
