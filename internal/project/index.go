package project

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

var recognizedSections = map[string]CatalogSection{
	string(SectionProjectReasoningPolicy):     SectionProjectReasoningPolicy,
	string(SectionProjectReasoningTemplates):  SectionProjectReasoningTemplates,
	string(SectionSourceDocuments):            SectionSourceDocuments,
	string(SectionActiveContractPool):         SectionActiveContractPool,
	string(SectionAvailableEvidenceReports):   SectionAvailableEvidenceReports,
	string(SectionAvailableReasoningTemplate): SectionAvailableReasoningTemplate,
	string(SectionReviewProtocols):            SectionReviewProtocols,
	string(SectionSmokeHarnesses):             SectionSmokeHarnesses,
}

func ParseProjectIndex(content string) (ProjectIndex, []ValidationFinding) {
	var index ProjectIndex
	var findings []ValidationFinding
	var section CatalogSection
	var headers []string
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			headers = nil
			name := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			section = recognizedSections[name]
			continue
		}
		if section == "" || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := parseTableRow(trimmed)
		if len(cells) == 0 || isSeparatorRow(cells) {
			continue
		}
		if headers == nil {
			headers = cells
			continue
		}
		if section == SectionProjectReasoningPolicy {
			row := rowMap(headers, cells)
			setting, value := strings.ToLower(trimInlineCode(row["setting"])), strings.ToLower(trimInlineCode(row["value"]))
			switch setting {
			case "mode":
				if value != string(ProjectReasoningOptional) && value != string(ProjectReasoningRequired) {
					findings = append(findings, ValidationFinding{Severity: SeverityError, Section: section, Problem: "invalid project reasoning mode", Cause: fmt.Sprintf("line %d: expected optional or required", i+1), Suggestion: "Set Mode to optional or required."})
				} else {
					index.ProjectReasoningPolicy.Mode = ProjectReasoningMode(value)
				}
			case "required review verdict":
				index.ProjectReasoningPolicy.RequiredReviewVerdict = value
			default:
				findings = append(findings, ValidationFinding{Severity: SeverityError, Section: section, Problem: "unknown project reasoning setting", Cause: fmt.Sprintf("line %d: %s", i+1, setting), Suggestion: "Use Mode or Required Review Verdict."})
			}
			continue
		}
		entry, err := entryFromRow(section, headers, cells)
		if err != nil {
			findings = append(findings, ValidationFinding{
				Severity:   SeverityError,
				Section:    section,
				Problem:    "malformed catalog row",
				Cause:      fmt.Sprintf("line %d: %s", i+1, err),
				Suggestion: "Fix the table row so it includes the required name and path columns.",
			})
			continue
		}
		index.Entries = append(index.Entries, entry)
	}
	return index, findings
}

func rowMap(headers, cells []string) map[string]string {
	row := map[string]string{}
	for i, h := range headers {
		if i < len(cells) {
			row[strings.ToLower(strings.TrimSpace(h))] = cells[i]
		}
	}
	return row
}

func entryFromRow(section CatalogSection, headers, cells []string) (CatalogEntry, error) {
	row := rowMap(headers, cells)
	name := firstNonEmpty(row["document"], row["contract"], row["report"], row["template"], row["protocol"], row["decision"], row["harness"])
	path := firstNonEmpty(row["path"], row["output path"])
	if name == "" {
		return CatalogEntry{}, fmt.Errorf("missing entry name")
	}
	if path == "" || strings.EqualFold(path, "N/A") {
		return CatalogEntry{}, fmt.Errorf("missing path")
	}
	entry := CatalogEntry{
		Section:     section,
		Name:        trimInlineCode(name),
		Path:        trimInlineCode(path),
		Description: firstNonEmpty(row["summary"], row["covers"], row["applies to"], row["useful for"], row["required when"], row["why selected"]),
	}
	entry.Manifest = trimInlineCode(row["manifest"])
	entry.Status = trimInlineCode(row["status"])
	if evidence := trimInlineCode(row["evidence"]); evidence != "" {
		for _, value := range strings.Split(evidence, " and ") {
			value = strings.TrimSpace(strings.Trim(value, "`"))
			if value != "" {
				entry.Evidence = append(entry.Evidence, value)
			}
		}
	}
	entry.External = isExternalPath(entry.Path) || (section == SectionSmokeHarnesses && filepath.IsAbs(entry.Path))
	if section == SectionSmokeHarnesses && entry.Manifest == "" {
		return CatalogEntry{}, fmt.Errorf("missing manifest")
	}
	return entry, nil
}

func parseTableRow(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func isSeparatorRow(cells []string) bool {
	for _, cell := range cells {
		cell = strings.TrimSpace(cell)
		if cell == "" {
			return false
		}
		for _, r := range cell {
			if r != '-' && r != ':' {
				return false
			}
		}
	}
	return true
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func trimInlineCode(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`")
	if strings.HasPrefix(value, "[") && strings.Contains(value, "](") && strings.HasSuffix(value, ")") {
		start := strings.Index(value, "](")
		return value[start+2 : len(value)-1]
	}
	return strings.TrimSpace(value)
}

func isExternalPath(path string) bool {
	u, err := url.Parse(path)
	return err == nil && u.Scheme != "" && u.Host != ""
}
