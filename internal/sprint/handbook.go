package sprint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/project"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type EvidenceEntry struct {
	Name    string
	Path    string
	RelPath string
}

type HandbookManifest struct {
	ProjectSlug      string
	SprintSlug       string
	SprintRoot       string
	RequirementsPath string
	ProjectIndexPath string
	SprintIndexPath  string
	OutputPath       string
	Evidence         []EvidenceEntry
}

func BuildHandbookManifest(root string, sp Sprint, inputs PlanningInputs, catalog project.ProjectIndex) (HandbookManifest, []ValidationFinding) {
	index, findings := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
	manifest := HandbookManifest{
		ProjectSlug:      sp.Project,
		SprintSlug:       sp.Slug,
		SprintRoot:       filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug)),
		RequirementsPath: ArtifactRelPath(sp, StageRequirements),
		ProjectIndexPath: filepath.ToSlash(filepath.Join("projects", sp.Project, "project-index.md")),
		SprintIndexPath:  ArtifactRelPath(sp, StageSprintIndex),
		OutputPath:       ArtifactRelPath(sp, StageTechnicalHandbook),
	}
	seen := map[string]bool{}
	for _, selected := range index.EvidenceReports {
		key := strings.ToLower(selected.Name + "\x00" + selected.Path)
		if seen[key] {
			findings = append(findings, finding("Selected Evidence Reports", selected.Name, selected.Path, "duplicate selected evidence", "same evidence report appears more than once", "Remove the duplicate selected evidence row."))
			continue
		}
		seen[key] = true
		entry, ok := catalogEvidence(catalog, selected)
		if !ok {
			continue
		}
		rel := normalizeWorkspacePath(entry.Path)
		if filepath.IsAbs(rel) || strings.HasPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))), "../") {
			findings = append(findings, finding("Selected Evidence Reports", selected.Name, selected.Path, "unsafe evidence path", "path escapes the workspace", "Use a workspace-relative catalog path."))
			continue
		}
		full, err := workspace.ResolveInside(root, rel)
		if err != nil {
			findings = append(findings, finding("Selected Evidence Reports", selected.Name, selected.Path, "unsafe evidence path", err.Error(), "Use a workspace-relative catalog path."))
			continue
		}
		info, err := os.Stat(full)
		if err != nil {
			findings = append(findings, finding("Selected Evidence Reports", selected.Name, selected.Path, "unreadable selected evidence", err.Error(), "Ensure the selected report exists and is readable."))
			continue
		}
		if info.IsDir() || info.Size() == 0 {
			findings = append(findings, finding("Selected Evidence Reports", selected.Name, selected.Path, "invalid selected evidence file", "path is empty or a directory", "Select a non-empty Markdown evidence report."))
			continue
		}
		manifest.Evidence = append(manifest.Evidence, EvidenceEntry{Name: selected.Name, Path: selected.Path, RelPath: rel})
	}
	sort.SliceStable(manifest.Evidence, func(i, j int) bool {
		if manifest.Evidence[i].Name != manifest.Evidence[j].Name {
			return manifest.Evidence[i].Name < manifest.Evidence[j].Name
		}
		return manifest.Evidence[i].Path < manifest.Evidence[j].Path
	})
	sortSprintFindings(findings)
	return manifest, findings
}

func ValidateTechnicalHandbookContent(content string, manifest HandbookManifest) []ValidationFinding {
	var findings []ValidationFinding
	if strings.TrimSpace(content) == "" {
		return []ValidationFinding{finding("technical-handbook.md", "", "", "empty technical handbook", "file has no content", "Generate or write the required handbook sections.")}
	}
	if containsPlaceholder(content) {
		findings = append(findings, finding("technical-handbook.md", "", "", "placeholder content", "file still contains placeholder markers", "Replace placeholders with concrete handbook content."))
	}
	sections := markdownSections(content)
	required := []string{"Selected Studies And Reports", "Relevant Patterns", "Trade-Offs", "Anti-Patterns And Warnings", "Open Questions For Reasoning", "Evidence Pointers"}
	for _, section := range required {
		if strings.TrimSpace(sections[section]) == "" {
			findings = append(findings, finding(section, "", "", "missing required section", "section was not found or has no content", "Add evidence-backed handbook content for this section."))
		}
	}
	traceText := sections["Selected Studies And Reports"] + "\n" + sections["Evidence Pointers"]
	for _, evidence := range manifest.Evidence {
		if !strings.Contains(traceText, evidence.Name) && !strings.Contains(traceText, evidence.Path) && !strings.Contains(traceText, evidence.RelPath) {
			findings = append(findings, finding("Evidence Pointers", evidence.Name, evidence.Path, "missing selected evidence trace", "selected evidence is not cited in handbook evidence sections", "Cite each selected report name or selected report path."))
		}
	}
	allowed := map[string]bool{}
	for _, evidence := range manifest.Evidence {
		allowed[strings.ToLower(evidence.Name)] = true
		allowed[strings.ToLower(evidence.Path)] = true
		allowed[strings.ToLower(evidence.RelPath)] = true
	}
	for _, path := range markdownPaths(content) {
		lp := strings.ToLower(path)
		normalized := strings.ToLower(normalizeWorkspacePath(path))
		if strings.Contains(lp, "/reports/final/") && !allowed[lp] && !allowed[normalized] {
			findings = append(findings, finding("Evidence Pointers", "", path, "unselected evidence reference", "handbook cites a report that is not selected by sprint-index.md", "Use only selected evidence reports."))
		}
	}
	for heading := range sections {
		lower := strings.ToLower(heading)
		if strings.Contains(lower, "decision") || strings.Contains(lower, "implementation plan") || strings.Contains(lower, "task plan") {
			findings = append(findings, finding(heading, "", "", "decision-language section", "technical-handbook.md must distill evidence, not make final decisions", "Move final decisions to reasoning.md or plan.md."))
		}
	}
	for _, phrase := range []string{"final decision:", "we will implement", "implementation decision", "decision:"} {
		if strings.Contains(strings.ToLower(content), phrase) {
			findings = append(findings, finding("technical-handbook.md", "", "", "implementation decision language", "handbook contains final decision wording", "Frame handbook content as evidence, trade-offs, warnings, or open questions."))
			break
		}
	}
	sortSprintFindings(findings)
	return findings
}

func catalogEvidence(catalog project.ProjectIndex, selected SelectedItem) (project.CatalogEntry, bool) {
	for _, entry := range catalog.Entries {
		if entry.Section == project.SectionAvailableEvidenceReports && strings.EqualFold(entry.Name, selected.Name) && entry.Path == selected.Path {
			return entry, true
		}
	}
	return project.CatalogEntry{}, false
}

func normalizeWorkspacePath(path string) string {
	path = strings.Trim(path, "`")
	path = filepath.ToSlash(path)
	if strings.HasPrefix(path, "ultra/") {
		path = "." + path
	}
	return strings.TrimPrefix(path, ".ultra/")
}

func markdownSections(content string) map[string]string {
	sections := map[string]string{}
	current := ""
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			current = strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			sections[current] = ""
			continue
		}
		if current != "" {
			sections[current] += line + "\n"
		}
	}
	return sections
}

var mdPathPattern = regexp.MustCompile(`(?:\.ultra/)?[A-Za-z0-9._/-]+/reports/final/[A-Za-z0-9._/-]+\.md`)

func markdownPaths(content string) []string {
	matches := mdPathPattern.FindAllString(content, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, match := range matches {
		match = strings.Trim(match, "`.,);")
		if strings.HasPrefix(match, "ultra/") {
			match = "." + match
		}
		if !seen[match] {
			seen[match] = true
			out = append(out, match)
		}
	}
	sort.Strings(out)
	return out
}

func formatManifest(manifest HandbookManifest) string {
	var b strings.Builder
	fmt.Fprintf(&b, "- Project: %s\n", manifest.ProjectSlug)
	fmt.Fprintf(&b, "- Sprint: %s\n", manifest.SprintSlug)
	fmt.Fprintf(&b, "- Sprint root: %s\n", manifest.SprintRoot)
	fmt.Fprintf(&b, "- Requirements: %s\n", manifest.RequirementsPath)
	fmt.Fprintf(&b, "- Project index: %s\n", manifest.ProjectIndexPath)
	fmt.Fprintf(&b, "- Sprint index: %s\n", manifest.SprintIndexPath)
	fmt.Fprintf(&b, "- Output: %s\n", manifest.OutputPath)
	fmt.Fprintln(&b, "- Selected evidence:")
	for _, evidence := range manifest.Evidence {
		fmt.Fprintf(&b, "  - %s: %s\n", evidence.Name, evidence.Path)
	}
	return b.String()
}
