package project

import (
	"fmt"
	"net/url"
	"strings"
)

var recognizedSections = map[string]CatalogSection{
	string(SectionSourceDocuments):            SectionSourceDocuments,
	string(SectionActiveContractPool):         SectionActiveContractPool,
	string(SectionAvailableEvidenceReports):   SectionAvailableEvidenceReports,
	string(SectionAvailableReasoningTemplate): SectionAvailableReasoningTemplate,
	string(SectionReviewProtocols):            SectionReviewProtocols,
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

func entryFromRow(section CatalogSection, headers, cells []string) (CatalogEntry, error) {
	row := map[string]string{}
	for i, h := range headers {
		if i < len(cells) {
			row[strings.ToLower(strings.TrimSpace(h))] = cells[i]
		}
	}
	name := firstNonEmpty(row["document"], row["contract"], row["report"], row["template"], row["protocol"], row["decision"])
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
	entry.External = isExternalPath(entry.Path)
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
