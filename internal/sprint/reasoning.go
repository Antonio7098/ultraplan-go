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

type ReasoningTemplateEntry struct {
	Name       string
	Template   string
	OutputPath string
	Why        string
}

type ReasoningManifest struct {
	ProjectSlug        string
	SprintSlug         string
	SprintRoot         string
	RequirementsPath   string
	SprintIndexPath    string
	HandbookPath       string
	FinalOutputPath    string
	Contracts          []SelectedItem
	EvidenceReports    []SelectedItem
	ReviewProtocols    []SelectedItem
	ExcludedContexts   []SelectedItem
	ReasoningTemplates []ReasoningTemplateEntry
}

func BuildReasoningManifest(root string, sp Sprint, inputs PlanningInputs, catalog project.ProjectIndex) (ReasoningManifest, []ValidationFinding) {
	index, findings := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
	manifest := ReasoningManifest{
		ProjectSlug:      sp.Project,
		SprintSlug:       sp.Slug,
		SprintRoot:       filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug)),
		RequirementsPath: ArtifactRelPath(sp, StageRequirements),
		SprintIndexPath:  ArtifactRelPath(sp, StageSprintIndex),
		HandbookPath:     ArtifactRelPath(sp, StageTechnicalHandbook),
		FinalOutputPath:  ArtifactRelPath(sp, StageReasoning),
		Contracts:        append([]SelectedItem(nil), index.Contracts...),
		EvidenceReports:  append([]SelectedItem(nil), index.EvidenceReports...),
		ReviewProtocols:  append([]SelectedItem(nil), index.ReviewProtocols...),
		ExcludedContexts: append([]SelectedItem(nil), index.ExcludedContexts...),
	}
	for _, selected := range index.ReasoningTemplates {
		catalogEntry, ok := catalogReasoningTemplate(catalog, selected.Name)
		if !ok {
			continue
		}
		templateRel := normalizeWorkspacePath(catalogEntry.Path)
		if !safeWorkspaceRel(templateRel) {
			findings = append(findings, finding("Selected Reasoning Templates", selected.Name, catalogEntry.Path, "unsafe template path", "template path escapes the workspace", "Use a workspace-relative catalog path."))
			continue
		}
		full, err := workspace.ResolveInside(root, templateRel)
		if err != nil {
			findings = append(findings, finding("Selected Reasoning Templates", selected.Name, catalogEntry.Path, "unsafe template path", err.Error(), "Use a workspace-relative catalog path."))
			continue
		}
		info, err := os.Stat(full)
		if err != nil {
			findings = append(findings, finding("Selected Reasoning Templates", selected.Name, catalogEntry.Path, "unreadable selected template", err.Error(), "Ensure the selected reasoning template exists and is readable."))
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			findings = append(findings, finding("Selected Reasoning Templates", selected.Name, catalogEntry.Path, "invalid selected template file", "path is empty or a directory", "Select a non-empty Markdown reasoning template."))
			continue
		}
		if !reasoningOutputInsideSprint(sp, selected.Path) {
			findings = append(findings, finding("Selected Reasoning Templates", selected.Name, selected.Path, "unsafe output path", "output path must be under the selected sprint reasoning directory", "Use projects/<project>/sprints/<sprint>/reasoning/<area>.md."))
			continue
		}
		manifest.ReasoningTemplates = append(manifest.ReasoningTemplates, ReasoningTemplateEntry{
			Name:       selected.Name,
			Template:   catalogEntry.Path,
			OutputPath: selected.Path,
			Why:        selected.Why,
		})
	}
	sort.SliceStable(manifest.ReasoningTemplates, func(i, j int) bool {
		if manifest.ReasoningTemplates[i].Name != manifest.ReasoningTemplates[j].Name {
			return manifest.ReasoningTemplates[i].Name < manifest.ReasoningTemplates[j].Name
		}
		return manifest.ReasoningTemplates[i].OutputPath < manifest.ReasoningTemplates[j].OutputPath
	})
	sortSprintFindings(findings)
	return manifest, findings
}

func ValidateAreaReasoningContent(content string, entry ReasoningTemplateEntry, manifest ReasoningManifest) []ValidationFinding {
	var findings []ValidationFinding
	if strings.TrimSpace(content) == "" {
		return []ValidationFinding{finding("area-reasoning", entry.Name, entry.OutputPath, "empty area reasoning", "file has no content", "Generate or write the required area reasoning sections.")}
	}
	if containsPlaceholder(content) {
		findings = append(findings, finding("area-reasoning", entry.Name, entry.OutputPath, "placeholder content", "file still contains placeholder markers", "Replace placeholders with concrete reasoning content."))
	}
	sections := markdownSections(content)
	for _, section := range []string{"Area Decisions", "Trade-Offs", "Evidence", "Risks"} {
		if strings.TrimSpace(sections[section]) == "" {
			findings = append(findings, finding(section, entry.Name, entry.OutputPath, "missing required section", "section was not found or has no content", "Add the required area reasoning section."))
		}
	}
	if !strings.Contains(content, entry.Name) && !strings.Contains(content, entry.Template) {
		findings = append(findings, finding("Evidence", entry.Name, entry.OutputPath, "missing selected template trace", "area reasoning does not name the selected template", "Reference the selected area/template being reasoned through."))
	}
	findings = append(findings, validateSelectedContextReferences(content, manifest)...)
	sortSprintFindings(findings)
	return findings
}

func ValidateFinalReasoningContent(content string, manifest ReasoningManifest) []ValidationFinding {
	var findings []ValidationFinding
	if strings.TrimSpace(content) == "" {
		return []ValidationFinding{finding("reasoning.md", "", manifest.FinalOutputPath, "empty final reasoning", "file has no content", "Generate or write the required final reasoning sections.")}
	}
	if containsPlaceholder(content) {
		findings = append(findings, finding("reasoning.md", "", manifest.FinalOutputPath, "placeholder content", "file still contains placeholder markers", "Replace placeholders with concrete reasoning content."))
	}
	sections := markdownSections(content)
	for _, section := range []string{"Decisions", "Expected Evidence", "Assumptions And Risks", "Implementation Constraints"} {
		if strings.TrimSpace(sections[section]) == "" {
			findings = append(findings, finding(section, "", manifest.FinalOutputPath, "missing required section", "section was not found or has no content", "Add the required final reasoning section."))
		}
	}
	for _, entry := range manifest.ReasoningTemplates {
		if !strings.Contains(content, entry.OutputPath) && !strings.Contains(content, entry.Name) {
			findings = append(findings, finding("Area-Specific Reasoning Inputs", entry.Name, entry.OutputPath, "missing selected area reference", "final reasoning does not reference a selected area reasoning artifact", "Reference each selected area reasoning output before completing final reasoning."))
		}
	}
	findings = append(findings, validateSelectedContextReferences(content, manifest)...)
	sortSprintFindings(findings)
	return findings
}

func validateSelectedContextReferences(content string, manifest ReasoningManifest) []ValidationFinding {
	allowed := map[string]bool{}
	for _, item := range manifest.Contracts {
		allowed[strings.ToLower(item.Name)] = true
		allowed[strings.ToLower(item.Path)] = true
		allowed[strings.ToLower(normalizeWorkspacePath(item.Path))] = true
	}
	for _, item := range manifest.EvidenceReports {
		allowed[strings.ToLower(item.Name)] = true
		allowed[strings.ToLower(item.Path)] = true
		allowed[strings.ToLower(normalizeWorkspacePath(item.Path))] = true
	}
	for _, item := range manifest.ReviewProtocols {
		allowed[strings.ToLower(item.Name)] = true
		allowed[strings.ToLower(item.Path)] = true
		allowed[strings.ToLower(normalizeWorkspacePath(item.Path))] = true
	}
	for _, entry := range manifest.ReasoningTemplates {
		allowed[strings.ToLower(entry.Name)] = true
		allowed[strings.ToLower(entry.Template)] = true
		allowed[strings.ToLower(normalizeWorkspacePath(entry.Template))] = true
		allowed[strings.ToLower(entry.OutputPath)] = true
		allowed[strings.ToLower(normalizeWorkspacePath(entry.OutputPath))] = true
	}
	allowed[strings.ToLower(manifest.RequirementsPath)] = true
	allowed[strings.ToLower(manifest.SprintIndexPath)] = true
	allowed[strings.ToLower(manifest.HandbookPath)] = true
	allowed[strings.ToLower(manifest.FinalOutputPath)] = true
	var findings []ValidationFinding
	for _, path := range selectedContextPaths(content) {
		lp := strings.ToLower(path)
		normalized := strings.ToLower(normalizeWorkspacePath(path))
		if allowed[lp] || allowed[normalized] {
			continue
		}
		findings = append(findings, finding("Selected Context", "", path, "unselected context reference", "reasoning cites a path not selected for this sprint", "Use only selected sprint-index context, handbook evidence, and selected area reasoning paths."))
	}
	return findings
}

func selectedContextPaths(content string) []string {
	matches := mdPathPattern.FindAllString(content, -1)
	for _, needle := range []string{".ultra/system/contracts/", ".ultra/system/protocols/", ".ultra/system/reasoning/"} {
		start := 0
		for {
			idx := strings.Index(content[start:], needle)
			if idx < 0 {
				break
			}
			idx += start
			end := idx
			for end < len(content) && !strings.ContainsRune(" \t\r\n`|,);", rune(content[end])) {
				end++
			}
			matches = append(matches, strings.Trim(content[idx:end], "`.,);"))
			start = end
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, match := range matches {
		if match == "" || seen[match] {
			continue
		}
		seen[match] = true
		out = append(out, match)
	}
	sort.Strings(out)
	return out
}

func catalogReasoningTemplate(catalog project.ProjectIndex, name string) (project.CatalogEntry, bool) {
	for _, entry := range catalog.Entries {
		if entry.Section == project.SectionAvailableReasoningTemplate && strings.EqualFold(entry.Name, name) {
			return entry, true
		}
	}
	return project.CatalogEntry{}, false
}

func reasoningOutputInsideSprint(sp Sprint, output string) bool {
	if !safeWorkspaceRel(output) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(normalizeWorkspacePath(output))))
	prefix := filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug, "reasoning")) + "/"
	return strings.HasPrefix(clean, prefix) && strings.ToLower(filepath.Ext(clean)) == ".md"
}

func safeWorkspaceRel(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\x00") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

func formatReasoningManifest(manifest ReasoningManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Project: %s\n", manifest.ProjectSlug)
	fmt.Fprintf(&b, "- Sprint: %s\n", manifest.SprintSlug)
	fmt.Fprintf(&b, "- Sprint root: %s\n", manifest.SprintRoot)
	fmt.Fprintf(&b, "- Requirements: %s\n", manifest.RequirementsPath)
	fmt.Fprintf(&b, "- Sprint index: %s\n", manifest.SprintIndexPath)
	fmt.Fprintf(&b, "- Technical handbook: %s\n", manifest.HandbookPath)
	fmt.Fprintf(&b, "- Final reasoning output: %s\n", manifest.FinalOutputPath)
	fmt.Fprintln(&b, "- Selected contracts:")
	for _, item := range manifest.Contracts {
		fmt.Fprintf(&b, "  - %s\n", item.Name)
	}
	fmt.Fprintln(&b, "- Selected evidence:")
	for _, item := range manifest.EvidenceReports {
		fmt.Fprintf(&b, "  - %s: %s\n", item.Name, item.Path)
	}
	fmt.Fprintln(&b, "- Selected reasoning templates:")
	if len(manifest.ReasoningTemplates) == 0 {
		fmt.Fprintln(&b, "  - none")
	}
	for _, entry := range manifest.ReasoningTemplates {
		fmt.Fprintf(&b, "  - %s: template=%s output=%s\n", entry.Name, entry.Template, entry.OutputPath)
	}
	fmt.Fprintln(&b, "- Required review protocols:")
	for _, item := range manifest.ReviewProtocols {
		fmt.Fprintf(&b, "  - %s: %s\n", item.Name, item.Path)
	}
	return b.String()
}
