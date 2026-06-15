package sprint

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"ultraplan-go/internal/project"
	"ultraplan-go/internal/workspace"
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
