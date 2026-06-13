package codeextract

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Extract(req Request) (Result, error) {
	result := Result{Status: StatusOK}
	for _, reportPath := range req.Reports {
		report, err := extractReport(req.WorkspaceRoot, reportPath)
		if err != nil {
			return Result{}, err
		}
		result.Reports = append(result.Reports, report)
		for _, diagnostic := range report.Diagnostics {
			if diagnostic.Citation != "" || diagnostic.Reason != "" {
				result.Unresolved = append(result.Unresolved, diagnostic)
			}
		}
		for _, ref := range report.References {
			if ref.Unresolved != nil {
				result.Unresolved = append(result.Unresolved, *ref.Unresolved)
			}
		}
	}
	for _, report := range result.Reports {
		if len(report.Diagnostics) > 0 && len(report.Sources) == 0 && len(report.References) > 0 {
			result.Status = StatusValidation
			return result, nil
		}
		for _, diagnostic := range report.Diagnostics {
			if diagnostic.Reason == "duplicate source row" || diagnostic.Reason == "malformed source row" {
				result.Status = StatusValidation
				return result, nil
			}
		}
	}
	if len(result.Unresolved) > 0 {
		result.Status = StatusPartial
	}
	return result, nil
}

func extractReport(workspaceRoot, reportPath string) (ReportResult, error) {
	content, err := os.ReadFile(reportPath)
	if err != nil {
		return ReportResult{}, fmt.Errorf("read report %s: %w", reportPath, err)
	}
	report := ReportResult{Path: reportPath}
	sources, refs, diagnostics := parseReport(string(content), reportPath)
	if len(refs) > 0 && len(sources) == 0 {
		diagnostics = append(diagnostics, Diagnostic{ReportPath: reportPath, Reason: "missing source table"})
		report.Diagnostics = diagnostics
		for _, ref := range refs {
			report.References = append(report.References, Reference{
				ReportPath: reportPath,
				Citation:   ref.citation,
				CitedPath:  ref.path,
				LineSpec:   ref.lineSpec,
				Status:     "unresolved",
				Unresolved: &Diagnostic{ReportPath: reportPath, Citation: ref.citation, Path: ref.path, Reason: "missing source table"},
			})
		}
		return report, nil
	}
	r, resolvedSources, sourceDiagnostics := newResolver(workspaceRoot, reportPath, sources)
	report.Sources = resolvedSources
	report.Diagnostics = append(diagnostics, sourceDiagnostics...)
	for _, ref := range refs {
		reference := Reference{
			ReportPath: reportPath,
			Citation:   ref.citation,
			CitedPath:  ref.path,
			LineSpec:   ref.lineSpec,
			Status:     "resolved",
		}
		source, resolvedPath, diagnostic := r.resolve(ref.path)
		if diagnostic != nil {
			diagnostic.ReportPath = reportPath
			diagnostic.Citation = ref.citation
			reference.Status = "unresolved"
			reference.Unresolved = diagnostic
			report.References = append(report.References, reference)
			continue
		}
		lines, diagnostic := readSnippet(resolvedPath, ref.lines)
		if diagnostic != nil {
			diagnostic.ReportPath = reportPath
			diagnostic.SourceName = source.Name
			diagnostic.Citation = ref.citation
			diagnostic.Path = ref.path
			reference.Status = "unresolved"
			reference.SourceName = source.Name
			reference.ResolvedPath = resolvedPath
			reference.Unresolved = diagnostic
			report.References = append(report.References, reference)
			continue
		}
		reference.SourceName = source.Name
		reference.ResolvedPath = resolvedPath
		reference.Snippet = lines
		report.References = append(report.References, reference)
	}
	return report, nil
}

func readSnippet(path string, requested []int) ([]Line, *Diagnostic) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, &Diagnostic{Reason: fmt.Sprintf("read source file: %v", err)}
	}
	all := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	lines := make([]Line, 0, len(requested))
	for _, n := range requested {
		if n < 1 || n > len(all) {
			return nil, &Diagnostic{Reason: fmt.Sprintf("line %d out of range", n)}
		}
		lines = append(lines, Line{Number: n, Text: all[n-1]})
	}
	return lines, nil
}

func RenderText(w io.Writer, workspaceRoot string, result Result) error {
	for _, report := range result.Reports {
		fmt.Fprintf(w, "Report: %s\n", rel(workspaceRoot, report.Path))
		for _, ref := range report.References {
			fmt.Fprintf(w, "Reference: %s\n", ref.Citation)
			fmt.Fprintf(w, "  Source: %s\n", emptyDash(ref.SourceName))
			fmt.Fprintf(w, "  Path: %s\n", ref.CitedPath)
			fmt.Fprintf(w, "  Lines: %s\n", ref.LineSpec)
			if ref.ResolvedPath != "" {
				fmt.Fprintf(w, "  Resolved: %s\n", rel(workspaceRoot, ref.ResolvedPath))
			}
			if ref.Unresolved != nil {
				fmt.Fprintf(w, "  Unresolved: %s\n", ref.Unresolved.Reason)
				continue
			}
			fmt.Fprintln(w, "  Snippet:")
			for _, line := range ref.Snippet {
				fmt.Fprintf(w, "    %d: %s\n", line.Number, line.Text)
			}
		}
		if len(report.References) == 0 {
			fmt.Fprintln(w, "References: 0")
		}
	}
	fmt.Fprintf(w, "Unresolved: %d\n", len(result.Unresolved))
	for _, diagnostic := range result.Unresolved {
		fmt.Fprintf(w, "  %s", rel(workspaceRoot, diagnostic.ReportPath))
		if diagnostic.Citation != "" {
			fmt.Fprintf(w, " %s", diagnostic.Citation)
		}
		fmt.Fprintf(w, ": %s\n", diagnostic.Reason)
	}
	return nil
}

func RenderJSON(w io.Writer, result Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func rel(root, path string) string {
	if path == "" {
		return ""
	}
	if rel, err := filepath.Rel(root, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(path)
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}
