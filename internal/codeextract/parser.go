package codeextract

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var inlineCitationPattern = regexp.MustCompile("`([^`\\n]+:[0-9][0-9,\\-– ]*)`")

type parsedReference struct {
	citation string
	path     string
	lineSpec string
	lines    []int
}

func parseReport(content, reportPath string) ([]Source, []parsedReference, []Diagnostic) {
	refs, refDiagnostics := parseReferences(content, reportPath)
	sources, sourceDiagnostics := parseSources(content, reportPath)
	diagnostics := append(sourceDiagnostics, refDiagnostics...)
	return sources, refs, diagnostics
}

func parseReferences(content, reportPath string) ([]parsedReference, []Diagnostic) {
	matches := inlineCitationPattern.FindAllStringSubmatch(content, -1)
	refs := make([]parsedReference, 0, len(matches))
	var diagnostics []Diagnostic
	seen := map[string]struct{}{}
	for _, match := range matches {
		citation := match[1]
		if _, ok := seen[citation]; ok {
			continue
		}
		seen[citation] = struct{}{}
		path, spec, ok := strings.Cut(citation, ":")
		if !ok || strings.TrimSpace(path) == "" || strings.TrimSpace(spec) == "" {
			diagnostics = append(diagnostics, Diagnostic{ReportPath: reportPath, Citation: citation, Reason: "malformed citation"})
			continue
		}
		lines, normalized, err := parseLineSpec(spec)
		if err != nil {
			diagnostics = append(diagnostics, Diagnostic{ReportPath: reportPath, Citation: citation, Path: path, Reason: err.Error()})
			continue
		}
		refs = append(refs, parsedReference{
			citation: citation,
			path:     strings.TrimSpace(path),
			lineSpec: normalized,
			lines:    lines,
		})
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].citation < refs[j].citation })
	return refs, diagnostics
}

func parseLineSpec(raw string) ([]int, string, error) {
	spec := strings.ReplaceAll(strings.TrimSpace(raw), "–", "-")
	if strings.Contains(spec, "-") {
		parts := strings.Split(spec, "-")
		if len(parts) != 2 {
			return nil, "", fmt.Errorf("malformed line range %q", raw)
		}
		start, err := parsePositiveLine(parts[0])
		if err != nil {
			return nil, "", err
		}
		end, err := parsePositiveLine(parts[1])
		if err != nil {
			return nil, "", err
		}
		if end < start {
			return nil, "", fmt.Errorf("line range %q ends before it starts", raw)
		}
		lines := make([]int, 0, end-start+1)
		for i := start; i <= end; i++ {
			lines = append(lines, i)
		}
		return lines, fmt.Sprintf("%d-%d", start, end), nil
	}
	parts := strings.Split(spec, ",")
	lines := make([]int, 0, len(parts))
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		line, err := parsePositiveLine(part)
		if err != nil {
			return nil, "", err
		}
		lines = append(lines, line)
		normalized = append(normalized, strconv.Itoa(line))
	}
	return lines, strings.Join(normalized, ","), nil
}

func parsePositiveLine(raw string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return 0, fmt.Errorf("line numbers must be positive integers")
	}
	return n, nil
}

func parseSources(content, reportPath string) ([]Source, []Diagnostic) {
	var sources []Source
	var diagnostics []Diagnostic
	seen := map[string]struct{}{}
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
			continue
		}
		cells := splitMarkdownRow(trimmed)
		if len(cells) < 3 {
			continue
		}
		index, err := strconv.Atoi(strings.TrimSpace(cells[0]))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(cells[1])
		path := strings.Trim(strings.TrimSpace(cells[2]), "`")
		if name == "" || path == "" {
			diagnostics = append(diagnostics, Diagnostic{ReportPath: reportPath, Reason: "malformed source row"})
			continue
		}
		if _, ok := seen[name]; ok {
			diagnostics = append(diagnostics, Diagnostic{ReportPath: reportPath, SourceName: name, Reason: "duplicate source row"})
			continue
		}
		seen[name] = struct{}{}
		sources = append(sources, Source{Index: index, Name: name, Path: path})
	}
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Index == sources[j].Index {
			return sources[i].Name < sources[j].Name
		}
		return sources[i].Index < sources[j].Index
	})
	return sources, diagnostics
}

func splitMarkdownRow(row string) []string {
	trimmed := strings.Trim(row, "|")
	parts := strings.Split(trimmed, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
