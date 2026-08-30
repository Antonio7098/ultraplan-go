package sprint

import (
	"path/filepath"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/project"
)

func ValidateSprintIndexContent(content string, catalog project.ProjectIndex) (SprintIndex, []ValidationFinding) {
	index, findings := ParseSprintIndex(content)
	findings = append(findings, validateSubset(index.Contracts, catalog, project.SectionActiveContractPool, "Selected Contracts")...)
	findings = append(findings, validateSubset(index.EvidenceReports, catalog, project.SectionAvailableEvidenceReports, "Selected Evidence Reports")...)
	findings = append(findings, validateReasoningTemplates(index.ReasoningTemplates, catalog)...)
	findings = append(findings, validateSubset(index.ReviewProtocols, catalog, project.SectionReviewProtocols, "Required Review Protocols")...)
	findings = append(findings, validateExcluded(index.ExcludedContexts)...)
	if catalog.ProjectReasoningPolicy.Mode == project.ProjectReasoningRequired {
		if len(index.ProjectReasoning) == 0 {
			findings = append(findings, finding("Selected Project Reasoning", "", "", "missing selected project reasoning", "required project reasoning policy needs the accepted synthesis", "Select project-reasoning/reasoning.md and any current area documents that apply."))
		}
		hasSynthesis := false
		for _, item := range index.ProjectReasoning {
			clean := strings.TrimPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(item.Path))), "./")
			if strings.HasSuffix(clean, "/project-reasoning/reasoning.md") {
				hasSynthesis = true
			}
			if !strings.Contains(clean, "/project-reasoning/") || strings.Contains(clean, "..") || strings.HasPrefix(clean, "/") {
				findings = append(findings, finding("Selected Project Reasoning", item.Name, item.Path, "invalid project reasoning path", "selection is outside the accepted project-reasoning manifest", "Select the synthesis or an area document under this project's project-reasoning directory."))
			}
		}
		if !hasSynthesis {
			findings = append(findings, finding("Selected Project Reasoning", "Project synthesis", "", "missing project synthesis", "the final accepted project synthesis is mandatory", "Select projects/<project>/project-reasoning/reasoning.md."))
		}
	}
	sortSprintFindings(findings)
	return index, findings
}

func validateSubset(items []SelectedItem, catalog project.ProjectIndex, section project.CatalogSection, label string) []ValidationFinding {
	var findings []ValidationFinding
	for _, item := range items {
		var byName []project.CatalogEntry
		for _, entry := range catalog.Entries {
			if entry.Section == section && strings.EqualFold(entry.Name, item.Name) {
				byName = append(byName, entry)
			}
		}
		if len(byName) == 0 {
			findings = append(findings, finding(label, item.Name, item.Path, "selected entry not in project index", "no catalog entry with this name exists in the required section", "Select an entry listed in project-index.md."))
			continue
		}
		if item.Path == "" || section == project.SectionActiveContractPool {
			continue
		}
		foundPath := false
		for _, entry := range byName {
			if entry.Path == item.Path {
				foundPath = true
				break
			}
		}
		if !foundPath {
			findings = append(findings, finding(label, item.Name, item.Path, "selected path does not match project index", "catalog entry name exists but with a different path", "Use the exact path from project-index.md."))
		}
	}
	return findings
}

func validateReasoningTemplates(items []SelectedItem, catalog project.ProjectIndex) []ValidationFinding {
	var findings []ValidationFinding
	for _, item := range items {
		foundName := false
		for _, entry := range catalog.Entries {
			if entry.Section == project.SectionAvailableReasoningTemplate && strings.EqualFold(entry.Name, item.Name) {
				foundName = true
				break
			}
		}
		if !foundName {
			findings = append(findings, finding("Selected Reasoning Templates", item.Name, item.Path, "selected entry not in project index", "no catalog entry with this template name exists", "Select a reasoning template listed in project-index.md."))
			continue
		}
		if item.Path == "" {
			findings = append(findings, finding("Selected Reasoning Templates", item.Name, item.Path, "missing output path", "reasoning template selection must name its sprint output path", "Set Output Path to a workspace-relative sprint reasoning file."))
			continue
		}
		if strings.HasPrefix(item.Path, "/") || strings.Contains(item.Path, "\x00") || strings.Contains(item.Path, "..") {
			findings = append(findings, finding("Selected Reasoning Templates", item.Name, item.Path, "unsafe output path", "output path must be workspace-relative and contained in the sprint", "Use a workspace-relative path under the sprint reasoning directory."))
		}
	}
	return findings
}

func validateExcluded(items []SelectedItem) []ValidationFinding {
	if len(items) == 0 {
		return []ValidationFinding{finding("Excluded Context", "", "", "missing excluded context", "no out-of-scope behavior was recorded", "List excluded context so later stages do not infer extra scope.")}
	}
	needles := []string{"implementation", "smoke", "review", "issue", "git"}
	seen := map[string]bool{}
	for _, item := range items {
		text := strings.ToLower(item.Name + " " + item.Why)
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				seen[needle] = true
			}
		}
	}
	var findings []ValidationFinding
	for _, needle := range needles {
		if !seen[needle] {
			findings = append(findings, finding("Excluded Context", needle, "", "required excluded context not recorded", "known Phase 2 non-goal is not mentioned", "Record the deferred behavior in Excluded Context."))
		}
	}
	return findings
}
