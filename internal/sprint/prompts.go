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
