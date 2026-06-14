package sprint

import (
	"strings"

	"ultraplan-go/internal/project"
)

func ValidateSprintIndexContent(content string, catalog project.ProjectIndex) (SprintIndex, []ValidationFinding) {
	index, findings := ParseSprintIndex(content)
	findings = append(findings, validateSubset(index.Contracts, catalog, project.SectionActiveContractPool, "Selected Contracts")...)
	findings = append(findings, validateSubset(index.EvidenceReports, catalog, project.SectionAvailableEvidenceReports, "Selected Evidence Reports")...)
	findings = append(findings, validateSubset(index.ReasoningTemplates, catalog, project.SectionAvailableReasoningTemplate, "Selected Reasoning Templates")...)
	findings = append(findings, validateSubset(index.ReviewProtocols, catalog, project.SectionReviewProtocols, "Required Review Protocols")...)
	findings = append(findings, validateExcluded(index.ExcludedContexts)...)
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
