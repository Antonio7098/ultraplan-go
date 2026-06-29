package sprint

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type PromptPreview struct {
	Project string
	Sprint  string
	Prompt  string
}

func RenderSprintIndexPrompt(root string, sp Sprint, catalog project.ProjectIndex, docs []string) PromptPreview {
	sort.Strings(docs)
	var b strings.Builder
	fmt.Fprintf(&b, "Generate sprint-index.md for project %s sprint %s.\n\n", sp.Project, sp.Slug)
	fmt.Fprintln(&b, "Inputs:")
	fmt.Fprintf(&b, "- Requirements: %s\n", ArtifactRelPath(sp, StageRequirements))
	fmt.Fprintf(&b, "- Output: %s\n", ArtifactRelPath(sp, StageSprintIndex))
	fmt.Fprintf(&b, "- Project index: %s\n", filepath.ToSlash(filepath.Join("projects", sp.Project, "project-index.md")))
	fmt.Fprintf(&b, "- Roadmap: %s\n", filepath.ToSlash(filepath.Join("projects", sp.Project, "roadmap.md")))
	for _, doc := range docs {
		fmt.Fprintf(&b, "- Project doc: %s\n", filepath.ToSlash(filepath.Join("projects", sp.Project, doc)))
	}
	fmt.Fprintln(&b, "\nAvailable catalog entries:")
	writeCatalog(&b, catalog)
	fmt.Fprintln(&b, "\nRules:")
	fmt.Fprintln(&b, "- Select only entries listed in project-index.md.")
	fmt.Fprintln(&b, "- Use workspace-relative paths.")
	fmt.Fprintln(&b, "- Include Selected Contracts, Selected Evidence Reports, Selected Reasoning Templates, Required Review Protocols, and Excluded Context.")
	fmt.Fprintln(&b, "- Carry forward Phase 2 non-goals: no implementation execution, smoke investigation, review automation, issue tracking, Git mutation, or study-side execution semantics.")
	fmt.Fprintln(&b, "- Do not mutate project-index.md, roadmap.md, docs, source repositories, config, Git state, or any artifact other than sprint-index.md.")
	fmt.Fprintln(&b, "- This prompt preview is runtime-free and must not itself write artifacts.")
	prompt := strings.ReplaceAll(b.String(), root, workspace.Rel(root, root))
	return PromptPreview{Project: sp.Project, Sprint: sp.Slug, Prompt: prompt}
}

func RenderTechnicalHandbookPrompt(root string, manifest HandbookManifest) PromptPreview {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate technical-handbook.md for project %s sprint %s.\n\n", manifest.ProjectSlug, manifest.SprintSlug)
	fmt.Fprintln(&b, "Input manifest:")
	fmt.Fprint(&b, formatManifest(manifest))
	fmt.Fprintln(&b, "\nRequired sections:")
	fmt.Fprintln(&b, "- Selected Studies And Reports")
	fmt.Fprintln(&b, "- Relevant Patterns")
	fmt.Fprintln(&b, "- Trade-Offs")
	fmt.Fprintln(&b, "- Anti-Patterns And Warnings")
	fmt.Fprintln(&b, "- Open Questions For Reasoning")
	fmt.Fprintln(&b, "- Evidence Pointers")
	fmt.Fprintln(&b, "\nRules:")
	fmt.Fprintln(&b, "- Read and cite only the selected evidence reports in the manifest.")
	fmt.Fprintln(&b, "- Use workspace-relative paths in handbook citations.")
	fmt.Fprintln(&b, "- Distill observed patterns, trade-offs, warnings, examples, design pressures, and open questions.")
	fmt.Fprintln(&b, "- Do not make final architecture decisions, implementation decisions, task plans, or sprint plan sections.")
	fmt.Fprintln(&b, "- Write editable Markdown only to the output path.")
	fmt.Fprintln(&b, "- Do not mutate project-index.md, roadmap.md, docs, selected evidence reports, source repositories, config, Git state, implementation files, sprint-index.md, reasoning artifacts, or plan.md.")
	fmt.Fprintln(&b, "- This prompt preview is runtime-free and must not itself write artifacts.")
	prompt := strings.ReplaceAll(b.String(), root, workspace.Rel(root, root))
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}
}

func RenderAreaReasoningPrompt(root string, manifest ReasoningManifest, entry ReasoningTemplateEntry) PromptPreview {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate area reasoning for project %s sprint %s.\n\n", manifest.ProjectSlug, manifest.SprintSlug)
	fmt.Fprintln(&b, "Input manifest:")
	fmt.Fprint(&b, formatReasoningManifest(manifest))
	fmt.Fprintf(&b, "- Selected area template: %s\n", entry.Name)
	fmt.Fprintf(&b, "- Template path: %s\n", entry.Template)
	fmt.Fprintf(&b, "- Output: %s\n", entry.OutputPath)
	fmt.Fprintln(&b, "\nRequired sections:")
	fmt.Fprintln(&b, "- Area Decisions")
	fmt.Fprintln(&b, "- Trade-Offs")
	fmt.Fprintln(&b, "- Evidence")
	fmt.Fprintln(&b, "- Risks")
	fmt.Fprintln(&b, "\nRules:")
	fmt.Fprintln(&b, "- Use only selected context from sprint-index.md and technical-handbook.md.")
	fmt.Fprintln(&b, "- Use workspace-relative paths.")
	fmt.Fprintln(&b, "- Do not write final reasoning.md, plan.md, implementation files, smoke artifacts, review artifacts, issue artifacts, workspace config, source repositories, or Git state.")
	fmt.Fprintln(&b, "- Write editable Markdown only to the selected area output path.")
	fmt.Fprintln(&b, "- This prompt preview is runtime-free and must not itself write artifacts.")
	prompt := strings.ReplaceAll(b.String(), root, workspace.Rel(root, root))
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}
}

func RenderFinalReasoningPrompt(root string, manifest ReasoningManifest) PromptPreview {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate reasoning.md for project %s sprint %s.\n\n", manifest.ProjectSlug, manifest.SprintSlug)
	fmt.Fprintln(&b, "Input manifest:")
	fmt.Fprint(&b, formatReasoningManifest(manifest))
	fmt.Fprintln(&b, "\nRequired selected area reasoning:")
	if len(manifest.ReasoningTemplates) == 0 {
		fmt.Fprintln(&b, "- none; area-reasoning is skipped only because no templates are selected")
	}
	for _, entry := range manifest.ReasoningTemplates {
		fmt.Fprintf(&b, "- %s: %s\n", entry.Name, entry.OutputPath)
	}
	fmt.Fprintln(&b, "\nRequired sections:")
	fmt.Fprintln(&b, "- Sprint Purpose")
	fmt.Fprintln(&b, "- Selected Context And Pre-Reasoning Artifacts")
	fmt.Fprintln(&b, "- Area-Specific Reasoning Inputs")
	fmt.Fprintln(&b, "- Decisions")
	fmt.Fprintln(&b, "- Expected Evidence")
	fmt.Fprintln(&b, "- Assumptions And Risks")
	fmt.Fprintln(&b, "- Implementation Constraints")
	fmt.Fprintln(&b, "\nRules:")
	fmt.Fprintln(&b, "- Use only selected context from sprint-index.md, technical-handbook.md, and required selected area reasoning artifacts.")
	fmt.Fprintln(&b, "- Do not generate or validate plan.md, task checklists, implementation files, smoke artifacts, review artifacts, issue artifacts, workspace config, source repositories, or Git state.")
	fmt.Fprintln(&b, "- Write editable Markdown only to reasoning.md.")
	fmt.Fprintln(&b, "- This prompt preview is runtime-free and must not itself write artifacts.")
	prompt := strings.ReplaceAll(b.String(), root, workspace.Rel(root, root))
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}
}

func RenderPlanPrompt(root string, manifest PlanManifest) PromptPreview {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate plan.md for project %s sprint %s.\n\n", manifest.ProjectSlug, manifest.SprintSlug)
	fmt.Fprintln(&b, "Input manifest:")
	fmt.Fprintf(&b, "- Project: %s\n", manifest.ProjectSlug)
	fmt.Fprintf(&b, "- Sprint: %s\n", manifest.SprintSlug)
	fmt.Fprintf(&b, "- Sprint root: %s\n", manifest.SprintRoot)
	fmt.Fprintf(&b, "- Requirements: %s\n", manifest.RequirementsPath)
	fmt.Fprintf(&b, "- Sprint index: %s\n", manifest.SprintIndexPath)
	fmt.Fprintf(&b, "- Technical handbook: %s\n", manifest.HandbookPath)
	fmt.Fprintf(&b, "- Final reasoning: %s\n", manifest.ReasoningPath)
	fmt.Fprintf(&b, "- Output: %s\n", manifest.OutputPath)
	fmt.Fprintln(&b, "- Selected area reasoning:")
	if len(manifest.ReasoningTemplates) == 0 {
		fmt.Fprintln(&b, "  - none; area-reasoning is skipped only because no templates are selected")
	}
	for _, entry := range manifest.ReasoningTemplates {
		fmt.Fprintf(&b, "  - %s: %s\n", entry.Name, entry.OutputPath)
	}
	fmt.Fprintln(&b, "\nReasoning decisions to execute:")
	for _, decision := range manifest.DecisionNames {
		fmt.Fprintf(&b, "- %s\n", decision)
	}
	fmt.Fprintln(&b, "\nRequired sections:")
	fmt.Fprintln(&b, "- Reasoning Source")
	fmt.Fprintln(&b, "- Sprint Status")
	fmt.Fprintln(&b, "- Decisions To Execute")
	fmt.Fprintln(&b, "- Requirements / Contracts To Satisfy")
	fmt.Fprintln(&b, "- Tasks")
	fmt.Fprintln(&b, "- Evidence Checklist")
	fmt.Fprintln(&b, "- Risks And Blockers")
	fmt.Fprintln(&b, "- Execution Log")
	fmt.Fprintln(&b, "- Completion Criteria")
	fmt.Fprintln(&b, "\nTraceability rules:")
	fmt.Fprintln(&b, "- Trace every executable task to reasoning decisions or acceptance evidence.")
	fmt.Fprintln(&b, "- Include verification or evidence expectations for each implementation task.")
	fmt.Fprintln(&b, "- Keep decisions, requirements, risks, and evidence aligned with reasoning.md.")
	fmt.Fprintln(&b, "\nMutation and execution rules:")
	fmt.Fprintln(&b, "- Write editable Markdown only to the expected plan.md output path.")
	fmt.Fprintln(&b, "- Do not execute implementation tasks, smoke investigations, review automation, issue tracking, Git commands, or multi-stage implementation run loops.")
	fmt.Fprintln(&b, "- Do not create .run-state.json, smoke.md, smoke.json, generated review.md, issues.md, or issues.json.")
	fmt.Fprintln(&b, "- Do not modify requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md, reasoning.md, project docs, prior reviews, source repositories, implementation files, workspace config, or Git state.")
	fmt.Fprintln(&b, "- This prompt preview is runtime-free and must not itself write artifacts.")
	prompt := strings.ReplaceAll(b.String(), root, workspace.Rel(root, root))
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}
}

func writeCatalog(b *strings.Builder, catalog project.ProjectIndex) {
	entries := append([]project.CatalogEntry(nil), catalog.Entries...)
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Section != entries[j].Section {
			return entries[i].Section < entries[j].Section
		}
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Path < entries[j].Path
	})
	for _, entry := range entries {
		fmt.Fprintf(b, "- %s: %s (%s)\n", entry.Section, entry.Name, entry.Path)
	}
}
