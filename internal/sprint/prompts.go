package sprint

import (
	"fmt"
	"os"
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

func RenderRequirementsPrompt(root string, sp Sprint, catalog project.ProjectIndex, docs []string) PromptPreview {
	sort.Strings(docs)
	var b strings.Builder
	fmt.Fprintln(&b, "Inputs:")
	fmt.Fprintf(&b, "- Output: %s\n", ArtifactRelPath(sp, StageRequirements))
	fmt.Fprintf(&b, "- Project index: %s\n", filepath.ToSlash(filepath.Join("projects", sp.Project, "project-index.md")))
	fmt.Fprintf(&b, "- Roadmap: %s\n", filepath.ToSlash(filepath.Join("projects", sp.Project, "roadmap.md")))
	for _, doc := range docs {
		fmt.Fprintf(&b, "- Project doc: %s\n", filepath.ToSlash(filepath.Join("projects", sp.Project, doc)))
	}
	fmt.Fprintln(&b, "\nAvailable catalog entries:")
	writeCatalog(&b, catalog)
	appendInjectedWorkspaceFile(root, &b, "Requirements Template", "templates/requirements.md")
	fmt.Fprintln(&b, "\nHard constraints:")
	fmt.Fprintln(&b, "- Derive the sprint requirements from roadmap.md, project-index.md, and project docs.")
	fmt.Fprintln(&b, "- Use workspace-relative paths.")
	fmt.Fprintln(&b, "- Write editable Markdown only to requirements.md.")
	fmt.Fprintln(&b, "- Do not mutate project-index.md, roadmap.md, docs, source repositories, config, Git state, or any artifact other than requirements.md.")
	prompt := renderPromptFromDefault(root, "prompts/create-requirements.md", sp.Project, sp.Slug, b.String())
	return PromptPreview{Project: sp.Project, Sprint: sp.Slug, Prompt: prompt}
}

func RenderSprintIndexPrompt(root string, sp Sprint, catalog project.ProjectIndex, docs []string) PromptPreview {
	sort.Strings(docs)
	var b strings.Builder
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
	appendInjectedWorkspaceFile(root, &b, "Sprint Index Template", "templates/sprint-index.md")
	fmt.Fprintln(&b, "\nHard constraints:")
	fmt.Fprintln(&b, "- Select only entries listed in project-index.md.")
	fmt.Fprintln(&b, "- Use workspace-relative paths.")
	fmt.Fprintln(&b, "- Do not mutate project-index.md, roadmap.md, docs, source repositories, config, Git state, or any artifact other than sprint-index.md.")
	prompt := renderPromptFromDefault(root, "prompts/create-sprint-index.md", sp.Project, sp.Slug, b.String())
	return PromptPreview{Project: sp.Project, Sprint: sp.Slug, Prompt: prompt}
}

func RenderTechnicalHandbookPrompt(root string, manifest HandbookManifest) PromptPreview {
	var b strings.Builder
	fmt.Fprintln(&b, "Input manifest:")
	fmt.Fprint(&b, formatManifest(manifest))
	appendInjectedWorkspaceFile(root, &b, "Technical Handbook Template", "templates/technical-handbook.md")
	fmt.Fprintln(&b, "\nHard constraints:")
	fmt.Fprintln(&b, "- Read and cite only the selected evidence reports in the manifest.")
	fmt.Fprintln(&b, "- Use workspace-relative paths in handbook citations.")
	fmt.Fprintln(&b, "- Write editable Markdown only to the output path.")
	fmt.Fprintln(&b, "- Do not mutate project-index.md, roadmap.md, docs, selected evidence reports, source repositories, config, Git state, implementation files, sprint-index.md, reasoning artifacts, or plan.md.")
	prompt := renderPromptFromDefault(root, "prompts/create-technical-handbook.md", manifest.ProjectSlug, manifest.SprintSlug, b.String())
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}
}

func RenderAreaReasoningPrompt(root string, manifest ReasoningManifest, entry ReasoningTemplateEntry) (PromptPreview, error) {
	if _, err := project.ResolveReasoningDefault(root, manifest.ProjectSlug, project.AreaReasoningPromptPath); err != nil {
		return PromptPreview{}, err
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Input manifest:")
	fmt.Fprint(&b, formatReasoningManifest(manifest))
	fmt.Fprintf(&b, "- Selected area template: %s\n", entry.Name)
	fmt.Fprintf(&b, "- Template path: %s\n", entry.Template)
	fmt.Fprintf(&b, "- Output: %s\n", entry.OutputPath)
	appendInjectedReasoningTemplate(root, &b, entry)
	fmt.Fprintln(&b, "\nHard constraints:")
	fmt.Fprintln(&b, "- Use only selected context from sprint-index.md and technical-handbook.md.")
	fmt.Fprintln(&b, "- Use workspace-relative paths.")
	fmt.Fprintln(&b, "- Do not write final reasoning.md, plan.md, implementation files, smoke artifacts, review artifacts, issue artifacts, workspace config, source repositories, or Git state.")
	fmt.Fprintln(&b, "- Write editable Markdown only to the selected area output path.")
	prompt := renderPromptFromDefault(root, "prompts/create-area-reasoning.md", manifest.ProjectSlug, manifest.SprintSlug, b.String())
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}, nil
}

func RenderFinalReasoningPrompt(root string, manifest ReasoningManifest) (PromptPreview, error) {
	if _, err := project.ResolveReasoningDefault(root, manifest.ProjectSlug, project.FinalReasoningPromptPath); err != nil {
		return PromptPreview{}, err
	}
	if _, err := project.ResolveReasoningDefault(root, manifest.ProjectSlug, project.FinalReasoningTemplatePath); err != nil {
		return PromptPreview{}, err
	}
	var b strings.Builder
	fmt.Fprintln(&b, "Input manifest:")
	fmt.Fprint(&b, formatReasoningManifest(manifest))
	fmt.Fprintln(&b, "\nRequired selected area reasoning:")
	if len(manifest.ReasoningTemplates) == 0 {
		fmt.Fprintln(&b, "- none; area-reasoning is skipped only because no templates are selected")
	}
	for _, entry := range manifest.ReasoningTemplates {
		fmt.Fprintf(&b, "- %s: %s\n", entry.Name, entry.OutputPath)
	}
	appendInjectedProjectReasoningFile(root, manifest.ProjectSlug, &b, "Sprint Reasoning Template", project.FinalReasoningTemplatePath)
	fmt.Fprintln(&b, "\nHard constraints:")
	fmt.Fprintln(&b, "- Use only selected context from sprint-index.md, technical-handbook.md, and required selected area reasoning artifacts.")
	fmt.Fprintln(&b, "- Do not generate or validate plan.md, task checklists, implementation files, smoke artifacts, review artifacts, issue artifacts, workspace config, source repositories, or Git state.")
	fmt.Fprintln(&b, "- Write editable Markdown only to reasoning.md.")
	prompt := renderPromptFromDefault(root, "prompts/create-sprint-reasoning.md", manifest.ProjectSlug, manifest.SprintSlug, b.String())
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}, nil
}

func RenderPlanPrompt(root string, manifest PlanManifest) PromptPreview {
	var b strings.Builder
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
	appendInjectedWorkspaceFile(root, &b, "Sprint Plan Template", "templates/sprint-plan.md")
	fmt.Fprintln(&b, "\nHard constraints:")
	fmt.Fprintln(&b, "- Write editable Markdown only to the expected plan.md output path.")
	fmt.Fprintln(&b, "- Do not execute implementation tasks, smoke investigations, review automation, issue tracking, Git commands, or multi-stage implementation run loops.")
	fmt.Fprintln(&b, "- Do not create .run-state.json, smoke.md, smoke.json, generated review.md, issues.md, or issues.json.")
	fmt.Fprintln(&b, "- Do not modify requirements.md, sprint-index.md, technical-handbook.md, reasoning/*.md, reasoning.md, project docs, prior reviews, source repositories, implementation files, workspace config, or Git state.")
	prompt := renderPromptFromDefault(root, "prompts/plan-sprint.md", manifest.ProjectSlug, manifest.SprintSlug, b.String())
	return PromptPreview{Project: manifest.ProjectSlug, Sprint: manifest.SprintSlug, Prompt: prompt}
}

func renderPromptFromDefault(root, rel, projectSlug, sprintSlug, manifest string) string {
	body, source := projectReasoningPromptTemplate(root, projectSlug, rel)
	replacements := map[string]string{
		"{project}":     projectSlug,
		"{sprint-slug}": sprintSlug,
	}
	for old, newValue := range replacements {
		body = strings.ReplaceAll(body, old, newValue)
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	fmt.Fprintln(&b, "\n\n---")
	fmt.Fprintln(&b, "\n## UltraPlan Runtime Manifest")
	fmt.Fprintf(&b, "\nPrompt source: `%s`\n", source)
	fmt.Fprintln(&b, "\nPath convention: this workspace uses paths relative to the workspace root. When prototype instructions mention `.ultra/...`, use the same path without the `.ultra/` prefix.")
	fmt.Fprintln(&b, "\nUse the concrete paths and selections below when they differ from placeholder text in the default prompt.")
	fmt.Fprintln(&b)
	b.WriteString(strings.ReplaceAll(strings.TrimRight(manifest, "\n"), root, workspace.Rel(root, root)))
	b.WriteString("\n")
	return b.String()
}

func sprintPromptTemplate(root, rel string) (string, string) {
	rel = filepath.ToSlash(rel)
	full, err := workspace.ResolveInside(root, rel)
	if err == nil {
		content, readErr := os.ReadFile(full)
		if readErr == nil {
			return string(content), rel
		}
		if !os.IsNotExist(readErr) {
			return fmt.Sprintf("# Prompt Load Error\n\nCould not read `%s`: %v\n", rel, readErr), rel
		}
	}
	if content, ok := workspace.DefaultOverrideFile(rel); ok {
		return content, "builtin:" + rel
	}
	return fmt.Sprintf("# Missing Prompt Default\n\nNo workspace override or built-in default exists for `%s`.\n", rel), rel
}

func projectReasoningPromptTemplate(root, projectSlug, rel string) (string, string) {
	if project.IsReasoningDefault(rel) {
		resolved, err := project.ResolveReasoningDefault(root, projectSlug, rel)
		if err != nil {
			return fmt.Sprintf("# Prompt Load Error\n\nCould not resolve `%s`: %v\n", rel, err), "invalid:" + rel
		}
		return resolved.Content, resolved.Source
	}
	return sprintPromptTemplate(root, rel)
}

func appendInjectedWorkspaceFile(root string, b *strings.Builder, label, rel string) {
	content, source := sprintPromptTemplate(root, rel)
	fmt.Fprintf(b, "\nInjected %s:\n", label)
	fmt.Fprintf(b, "Source: %s\n\n", source)
	fmt.Fprintln(b, strings.TrimRight(content, "\n"))
}

func appendInjectedProjectReasoningFile(root, projectSlug string, b *strings.Builder, label, rel string) {
	content, source := projectReasoningPromptTemplate(root, projectSlug, rel)
	fmt.Fprintf(b, "\nInjected %s:\n", label)
	fmt.Fprintf(b, "Source: %s\n\n", source)
	fmt.Fprintln(b, strings.TrimRight(content, "\n"))
}

func appendInjectedReasoningTemplate(root string, b *strings.Builder, entry ReasoningTemplateEntry) {
	rel := normalizeWorkspacePath(entry.Template)
	full, err := workspace.ResolveInside(root, rel)
	var content string
	source := entry.Template
	switch {
	case err != nil:
		content = fmt.Sprintf("Could not resolve selected reasoning template %s: %v", entry.Template, err)
	default:
		data, readErr := os.ReadFile(full)
		if readErr != nil {
			content = fmt.Sprintf("Could not read selected reasoning template %s: %v", entry.Template, readErr)
		} else {
			content = string(data)
			source = rel
		}
	}
	fmt.Fprintln(b, "\nInjected Selected Reasoning Template:")
	fmt.Fprintf(b, "Source: %s\n\n", source)
	fmt.Fprintln(b, strings.TrimRight(content, "\n"))
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
