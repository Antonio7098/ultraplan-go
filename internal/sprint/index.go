package sprint

import (
	"fmt"
	"sort"
	"strings"
)

type SprintIndex struct {
	Contracts          []SelectedItem
	EvidenceReports    []SelectedItem
	ReasoningTemplates []SelectedItem
	ReviewProtocols    []SelectedItem
	ExcludedContexts   []SelectedItem
	NoTemplates        bool
}

type SelectedItem struct {
	Name string
	Path string
	Why  string
}

var sprintIndexSections = map[string]string{
	"Selected Contracts":           "contracts",
	"Selected Evidence Reports":    "evidence",
	"Selected Reasoning Templates": "templates",
	"Required Review Protocols":    "protocols",
	"Excluded Context":             "excluded",
}

func ParseSprintIndex(content string) (SprintIndex, []ValidationFinding) {
	var index SprintIndex
	var findings []ValidationFinding
	if strings.TrimSpace(content) == "" {
		return index, []ValidationFinding{finding("sprint-index.md", "", "", "empty sprint-index.md", "file has no content", "Generate or write the required sprint-index.md sections.")}
	}
	if containsPlaceholder(content) {
		findings = append(findings, finding("sprint-index.md", "", "", "placeholder content", "file still contains placeholder markers", "Replace placeholders with concrete selected context."))
	}
	current := ""
	var headers []string
	seenSections := map[string]bool{}
	seenEntries := map[string]map[string]bool{}
	for i, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			headers = nil
			title := strings.TrimSpace(strings.TrimPrefix(trimmed, "## "))
			current = sprintIndexSections[title]
			if current != "" {
				seenSections[current] = true
			}
			continue
		}
		if current == "" {
			continue
		}
		if current == "templates" && strings.Contains(strings.ToLower(trimmed), "no reasoning templates") {
			index.NoTemplates = true
		}
		if !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := parseMarkdownRow(trimmed)
		if len(cells) == 0 || separatorRow(cells) {
			continue
		}
		if headers == nil {
			headers = lowerCells(cells)
			continue
		}
		item, err := selectedFromRow(current, headers, cells)
		if err != nil {
			findings = append(findings, finding(sectionName(current), "", "", "malformed table row", fmt.Sprintf("line %d: %s", i+1, err), "Fix the row so required name and path columns are present."))
			continue
		}
		if seenEntries[current] == nil {
			seenEntries[current] = map[string]bool{}
		}
		key := strings.ToLower(item.Name + "\x00" + item.Path)
		if seenEntries[current][key] {
			findings = append(findings, finding(sectionName(current), item.Name, item.Path, "duplicate selected entry", "same name/path appears more than once", "Remove the duplicate row."))
			continue
		}
		seenEntries[current][key] = true
		switch current {
		case "contracts":
			index.Contracts = append(index.Contracts, item)
		case "evidence":
			index.EvidenceReports = append(index.EvidenceReports, item)
		case "templates":
			index.ReasoningTemplates = append(index.ReasoningTemplates, item)
		case "protocols":
			index.ReviewProtocols = append(index.ReviewProtocols, item)
		case "excluded":
			index.ExcludedContexts = append(index.ExcludedContexts, item)
		}
	}
	for _, section := range []string{"contracts", "evidence", "templates", "protocols", "excluded"} {
		if !seenSections[section] {
			findings = append(findings, finding(sectionName(section), "", "", "missing required section", "section was not found", "Add the required sprint-index.md section."))
		}
	}
	if len(index.Contracts) == 0 {
		findings = append(findings, finding(sectionName("contracts"), "", "", "no selected contracts", "selected contracts table is empty", "Select at least one contract from project-index.md."))
	}
	if len(index.ReviewProtocols) == 0 {
		findings = append(findings, finding(sectionName("protocols"), "", "", "no review protocols", "required review protocols table is empty", "Select required review protocols from project-index.md."))
	}
	if len(index.ReasoningTemplates) == 0 && !index.NoTemplates {
		findings = append(findings, finding(sectionName("templates"), "", "", "no reasoning template selection", "no template rows and no explicit no-template selection were found", "Select templates or state that no reasoning templates are selected."))
	}
	sortSprintFindings(findings)
	return index, findings
}

func selectedFromRow(section string, headers, cells []string) (SelectedItem, error) {
	row := map[string]string{}
	for i, h := range headers {
		if i < len(cells) {
			row[h] = cells[i]
		}
	}
	item := SelectedItem{
		Name: trimCell(first(row["contract"], row["report"], row["template"], row["protocol"], row["context"], row["decision"])),
		Path: trimCell(first(row["path"], row["output path"])),
		Why:  trimCell(first(row["why selected"], row["covers"], row["required evidence"], row["reason excluded"], row["constraint for this sprint"])),
	}
	if item.Name == "" {
		return SelectedItem{}, fmt.Errorf("missing selected entry name")
	}
	if section != "contracts" && section != "excluded" && item.Path == "" {
		return SelectedItem{}, fmt.Errorf("missing path")
	}
	return item, nil
}

func parseMarkdownRow(line string) []string {
	parts := strings.Split(strings.Trim(line, "|"), "|")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func separatorRow(cells []string) bool {
	for _, cell := range cells {
		if cell == "" {
			return false
		}
		for _, r := range cell {
			if r != '-' && r != ':' && r != ' ' {
				return false
			}
		}
	}
	return true
}

func lowerCells(cells []string) []string {
	out := make([]string, len(cells))
	for i, cell := range cells {
		out[i] = strings.ToLower(strings.TrimSpace(cell))
	}
	return out
}

func trimCell(value string) string {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "`")
	if strings.EqualFold(value, "not applicable") || strings.EqualFold(value, "n/a") {
		return ""
	}
	return value
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func containsPlaceholder(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "todo") || strings.Contains(lower, "tbd") || strings.Contains(lower, "{{") || strings.Contains(lower, "<placeholder")
}

func sectionName(section string) string {
	switch section {
	case "contracts":
		return "Selected Contracts"
	case "evidence":
		return "Selected Evidence Reports"
	case "templates":
		return "Selected Reasoning Templates"
	case "protocols":
		return "Required Review Protocols"
	case "excluded":
		return "Excluded Context"
	default:
		return section
	}
}

func finding(section, entry, path, problem, cause, suggestion string) ValidationFinding {
	return ValidationFinding{Section: section, EntryName: entry, Path: path, Problem: problem, Cause: cause, Suggestion: suggestion}
}

func sortSprintFindings(findings []ValidationFinding) {
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Section != b.Section {
			return a.Section < b.Section
		}
		if a.EntryName != b.EntryName {
			return a.EntryName < b.EntryName
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Problem < b.Problem
	})
}
