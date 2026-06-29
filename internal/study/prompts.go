package study

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

var ErrPromptInapplicable = errors.New("prompt inapplicable")

func BuildAnalysisPrompt(req PromptRequest) (PromptResult, error) {
	if req.Source.Kind == SourceKindMarkdown && !SourceAppliesToDimension(req.Source, req.Dimension) {
		return PromptResult{}, fmt.Errorf("%w: markdown source %q does not apply to dimension %s", ErrPromptInapplicable, req.Source.Name, req.Dimension.Ref())
	}
	base, baseRel, err := readWorkspaceFile(req.WorkspaceRoot, "prompts/base.md")
	if err != nil {
		return PromptResult{}, err
	}
	dimension, err := readFile("dimension", req.Dimension.Path)
	if err != nil {
		return PromptResult{}, err
	}
	if req.Dimension.DisableCodeCitations || disablesCodeCitations(dimension) {
		req.Dimension.DisableCodeCitations = true
	}
	switch req.Source.Kind {
	case SourceKindDirectory:
		return buildDirectoryAnalysisPrompt(req, base, baseRel, dimension)
	case SourceKindMarkdown:
		return buildMarkdownAnalysisPrompt(req, base, baseRel, dimension)
	default:
		return PromptResult{}, fmt.Errorf("unsupported source kind %q for source %q", req.Source.Kind, req.Source.Name)
	}
}

func BuildSynthesisPrompt(req PromptRequest) (PromptResult, error) {
	synthesis, synthesisRel, err := readWorkspaceFile(req.WorkspaceRoot, "prompts/synthesize.md")
	if err != nil {
		return PromptResult{}, err
	}
	reportTemplate, reportRel, err := readWorkspaceFile(req.WorkspaceRoot, "templates/report.md")
	if err != nil {
		return PromptResult{}, err
	}
	dimension, err := readFile("dimension", req.Dimension.Path)
	if err != nil {
		return PromptResult{}, err
	}
	sources := req.Sources
	if len(sources) == 0 {
		discovered, err := DiscoverSources(req.Study)
		if err != nil {
			return PromptResult{}, err
		}
		sources = discovered
	}
	applicable := GetApplicableSources(sources, req.Dimension)
	sort.SliceStable(applicable, func(i, j int) bool {
		if applicable[i].Name == applicable[j].Name {
			return applicable[i].Kind < applicable[j].Kind
		}
		return applicable[i].Name < applicable[j].Name
	})
	var reports []SourceReportInput
	for _, source := range applicable {
		path := SourceReportPath(req.Study, source, req.Dimension)
		if _, err := os.Stat(path); err != nil {
			return PromptResult{}, fmt.Errorf("read source report for %s at %s: %w", source.Name, workspace.Rel(req.WorkspaceRoot, path), err)
		}
		reports = append(reports, SourceReportInput{
			Source:     source.Name,
			SourceKind: source.Kind,
			Path:       workspace.Rel(req.WorkspaceRoot, path),
		})
	}
	manifest := PromptManifest{
		Kind:               PromptKindSynthesis,
		Study:              req.Study.Name,
		Dimension:          req.Dimension.Ref(),
		Templates:          []string{synthesisRel, reportRel},
		DimensionPath:      workspace.Rel(req.WorkspaceRoot, req.Dimension.Path),
		SourceReports:      reports,
		InputReportPaths:   reportPaths(reports),
		ExpectedOutputPath: workspace.Rel(req.WorkspaceRoot, FinalReportPath(req.Study)),
	}
	text := joinSections(
		"Synthesis Prompt", synthesis,
		"Dimension", dimension,
		"Final Report Template", reportTemplate,
		"Synthesis Inputs", renderSourceReports(reports),
		"Output", fmt.Sprintf("Write the final report to %s.", manifest.ExpectedOutputPath),
	)
	return PromptResult{Text: text, Manifest: manifest}, nil
}

func buildDirectoryAnalysisPrompt(req PromptRequest, base, baseRel, dimension string) (PromptResult, error) {
	repoTemplate, repoRel, err := readWorkspaceFile(req.WorkspaceRoot, "templates/repo-analysis.md")
	if err != nil {
		return PromptResult{}, err
	}
	manifest := PromptManifest{
		Kind:               PromptKindDirectoryAnalysis,
		Study:              req.Study.Name,
		Dimension:          req.Dimension.Ref(),
		Source:             req.Source.Name,
		SourceKind:         req.Source.Kind,
		Templates:          []string{baseRel, repoRel},
		DimensionPath:      workspace.Rel(req.WorkspaceRoot, req.Dimension.Path),
		ExpectedOutputPath: workspace.Rel(req.WorkspaceRoot, SourceReportPath(req.Study, req.Source, req.Dimension)),
	}
	citationRule := "Cite code claims with workspace-relative file paths and line numbers from the selected source directory."
	if req.Dimension.DisableCodeCitations {
		citationRule = "Code citation requirements are disabled for this dimension."
	}
	text := joinSections(
		"Base Prompt", base,
		"Dimension", dimension,
		"Repository Analysis Template", repoTemplate,
		"Metadata", fmt.Sprintf("Study: %s\nSource: %s\nSource kind: %s\nSource path: %s\nExpected output: %s", req.Study.Name, req.Source.Name, req.Source.Kind, workspace.Rel(req.WorkspaceRoot, req.Source.Path), manifest.ExpectedOutputPath),
		"Source Isolation Rules", "Inspect only the selected source directory. Do not inspect unrelated workspace files, sibling sources, provider configuration, or generated reports except the explicit template and dimension inputs in this prompt.",
		"Citation Rules", citationRule,
	)
	return PromptResult{Text: text, Manifest: manifest}, nil
}

func buildMarkdownAnalysisPrompt(req PromptRequest, base, baseRel, dimension string) (PromptResult, error) {
	reportTemplate, reportRel, err := readWorkspaceFile(req.WorkspaceRoot, "templates/report.md")
	if err != nil {
		return PromptResult{}, err
	}
	document, err := readFile("markdown source document", req.Source.Path)
	if err != nil {
		return PromptResult{}, err
	}
	body := stripFrontmatter(document)
	manifest := PromptManifest{
		Kind:               PromptKindMarkdownAnalysis,
		Study:              req.Study.Name,
		Dimension:          req.Dimension.Ref(),
		Source:             req.Source.Name,
		SourceKind:         req.Source.Kind,
		Templates:          []string{baseRel, reportRel},
		DimensionPath:      workspace.Rel(req.WorkspaceRoot, req.Dimension.Path),
		InputDocumentPath:  workspace.Rel(req.WorkspaceRoot, req.Source.Path),
		ExpectedOutputPath: workspace.Rel(req.WorkspaceRoot, SourceReportPath(req.Study, req.Source, req.Dimension)),
	}
	text := joinSections(
		"Base Prompt", base,
		"Dimension", dimension,
		"Report Template", reportTemplate,
		"Metadata", fmt.Sprintf("Study: %s\nSource: %s\nSource kind: %s\nInput document: %s\nExpected output: %s", req.Study.Name, req.Source.Name, req.Source.Kind, manifest.InputDocumentPath, manifest.ExpectedOutputPath),
		"Document-Only Rules", "All source material is embedded below. Do not inspect external files, repositories, source code, sibling sources, provider configuration, or generated reports. Code citation requirements do not apply by default for Markdown document sources unless the dimension explicitly says otherwise.",
		"Embedded Document", body,
	)
	return PromptResult{Text: text, Manifest: manifest}, nil
}

func SourceAppliesToDimension(source Source, dimension Dimension) bool {
	if source.Kind == SourceKindDirectory || len(source.ApplicableDimensions) == 0 {
		return true
	}
	for _, applicable := range source.ApplicableDimensions {
		if applicable == dimension.Number {
			return true
		}
	}
	return false
}

func readWorkspaceFile(root, rel string) (string, string, error) {
	path, err := workspace.ResolveInside(root, rel)
	if err != nil {
		return "", "", err
	}
	content, err := readFile(rel, path)
	if err != nil {
		return "", "", err
	}
	return content, filepath.ToSlash(rel), nil
}

func readFile(label, path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s %s: %w", label, path, err)
	}
	return string(content), nil
}

func disablesCodeCitations(content string) bool {
	normalized := strings.ToLower(content)
	return strings.Contains(normalized, "disable code citations") ||
		strings.Contains(normalized, "code citations disabled") ||
		strings.Contains(normalized, "code citations are disabled")
}

func joinSections(parts ...string) string {
	var b strings.Builder
	for i := 0; i+1 < len(parts); i += 2 {
		fmt.Fprintf(&b, "## %s\n\n%s\n\n", parts[i], strings.TrimSpace(parts[i+1]))
	}
	return b.String()
}

func renderSourceReports(reports []SourceReportInput) string {
	if len(reports) == 0 {
		return "(none)"
	}
	var b strings.Builder
	for _, report := range reports {
		fmt.Fprintf(&b, "- %s [%s]: %s\n", report.Source, report.SourceKind, report.Path)
	}
	return strings.TrimRight(b.String(), "\n")
}

func reportPaths(reports []SourceReportInput) []string {
	paths := make([]string, 0, len(reports))
	for _, report := range reports {
		paths = append(paths, report.Path)
	}
	return paths
}
