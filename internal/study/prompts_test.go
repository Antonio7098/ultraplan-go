package study

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDirectoryAnalysisPromptManifestRulesAndDeterminism(t *testing.T) {
	root, st, dim, source := promptFixture(t)

	first, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	second, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if first.Text != second.Text {
		t.Fatal("prompt text is not deterministic")
	}
	assertManifestEqual(t, first.Manifest, second.Manifest)
	if first.Manifest.Kind != PromptKindDirectoryAnalysis {
		t.Fatalf("kind = %q", first.Manifest.Kind)
	}
	if first.Manifest.ExpectedOutputPath != filepath.Join(root, "studies", "demo", "reports", "source", "01-structure", "repo.md") {
		t.Fatalf("output = %q", first.Manifest.ExpectedOutputPath)
	}
	assertContains(t, first.Text, "# Base Prompt")
	assertContains(t, first.Text, "# Dimension")
	assertContains(t, first.Text, "# Repository Analysis")
	assertContains(t, first.Text, "Inspect only the selected source directory")
	assertContains(t, first.Text, "file paths and line numbers")
	assertContains(t, first.Text, first.Manifest.ExpectedOutputPath)
}

func TestBuildDirectoryAnalysisPromptDisabledCitationWording(t *testing.T) {
	root, st, dim, source := promptFixture(t)
	writeFileContent(t, st.Path, "# Dimension\n\nCode citations are disabled for this dimension.\n", "dimensions", "02-no-citations.md")
	dim = Dimension{Number: "02", Slug: "no-citations", File: "02-no-citations.md", Path: filepath.Join(st.Path, "dimensions", "02-no-citations.md")}

	result, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Text, "Code citation requirements are disabled")
}

func TestBuildDirectoryAnalysisPromptInapplicable(t *testing.T) {
	root, st, dim, source := promptFixture(t)
	source.ApplicableDimensions = []string{"02"}

	_, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: source})
	if !errors.Is(err, ErrPromptInapplicable) {
		t.Fatalf("err = %v, want ErrPromptInapplicable", err)
	}
}

func TestBuildMarkdownAnalysisPromptStripsFrontmatterAndDocumentOnly(t *testing.T) {
	root, st, dim, _ := promptFixture(t)
	source := Source{Name: "doc.md", Kind: SourceKindMarkdown, Path: filepath.Join(st.Path, "sources", "doc.md"), ApplicableDimensions: []string{"01"}}

	result, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: source})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Kind != PromptKindMarkdownAnalysis {
		t.Fatalf("kind = %q", result.Manifest.Kind)
	}
	if result.Manifest.InputDocumentPath != "studies/demo/sources/doc.md" {
		t.Fatalf("input doc = %q", result.Manifest.InputDocumentPath)
	}
	assertContains(t, result.Text, "# Document Body")
	assertNotContains(t, result.Text, "applicable_dimensions")
	assertContains(t, result.Text, "All source material is embedded below")
	assertContains(t, result.Text, "Do not inspect external files")
	assertContains(t, result.Text, "Code citation requirements do not apply by default")
}

func TestBuildMarkdownAnalysisPromptInapplicableAndMissingDocument(t *testing.T) {
	root, st, dim, _ := promptFixture(t)
	inapplicable := Source{Name: "doc.md", Kind: SourceKindMarkdown, Path: filepath.Join(st.Path, "sources", "doc.md"), ApplicableDimensions: []string{"02"}}
	_, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: inapplicable})
	if !errors.Is(err, ErrPromptInapplicable) {
		t.Fatalf("err = %v, want ErrPromptInapplicable", err)
	}

	missing := Source{Name: "missing.md", Kind: SourceKindMarkdown, Path: filepath.Join(st.Path, "sources", "missing.md")}
	_, err = BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: missing})
	if err == nil {
		t.Fatal("missing document error = nil")
	}
	assertContains(t, err.Error(), "missing.md")
}

func TestBuildSynthesisPromptApplicableReportsAndMissingFailures(t *testing.T) {
	root, st, dim, _ := promptFixture(t)
	writeFileContent(t, st.Path, "# Repo report\n", "reports", "source", "01-structure", "repo.md")
	writeFileContent(t, st.Path, "# Doc report\n", "reports", "source", "01-structure", "doc.md")
	writeFileContent(t, st.Path, "# Other doc report\n", "reports", "source", "01-structure", "other.md")

	result, err := BuildSynthesisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Kind != PromptKindSynthesis {
		t.Fatalf("kind = %q", result.Manifest.Kind)
	}
	assertContains(t, result.Text, "# Synthesis Prompt")
	assertContains(t, result.Text, "# Report")
	if len(result.Manifest.SourceReports) != 2 {
		t.Fatalf("reports = %+v", result.Manifest.SourceReports)
	}
	if result.Manifest.SourceReports[0].Source != "doc.md" || result.Manifest.SourceReports[1].Source != "repo" {
		t.Fatalf("report order/filter = %+v", result.Manifest.SourceReports)
	}

	if err := os.Remove(filepath.Join(st.Path, "reports", "source", "01-structure", "repo.md")); err != nil {
		t.Fatal(err)
	}
	_, err = BuildSynthesisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim})
	if err == nil {
		t.Fatal("missing report error = nil")
	}
	assertContains(t, err.Error(), "repo")
	assertContains(t, err.Error(), filepath.ToSlash(filepath.Join("01-structure", "repo.md")))
	assertNotContains(t, err.Error(), "other.md")
}

func TestBuildPromptUsesBuiltinDefaultsWhenWorkspaceOverridesAreMissing(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remove string
		build  func(root string, st Study, dim Dimension, source Source) error
	}{
		{name: "base", remove: "prompts/base.md", build: func(root string, st Study, dim Dimension, source Source) error {
			_, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: source})
			return err
		}},
		{name: "repo analysis", remove: "templates/repo-analysis.md", build: func(root string, st Study, dim Dimension, source Source) error {
			_, err := BuildAnalysisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim, Source: source})
			return err
		}},
		{name: "synthesize", remove: "prompts/synthesize.md", build: func(root string, st Study, dim Dimension, source Source) error {
			_, err := BuildSynthesisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim})
			return err
		}},
		{name: "final report template", remove: "templates/report.md", build: func(root string, st Study, dim Dimension, source Source) error {
			_, err := BuildSynthesisPrompt(PromptRequest{WorkspaceRoot: root, Study: st, Dimension: dim})
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, st, dim, source := promptFixture(t)
			if tc.name == "synthesize" || tc.name == "final report template" {
				writeFileContent(t, st.Path, "# Repo report\n", "reports", "source", "01-structure", "repo.md")
				writeFileContent(t, st.Path, "# Doc report\n", "reports", "source", "01-structure", "doc.md")
			}
			if err := os.Remove(filepath.Join(root, filepath.FromSlash(tc.remove))); err != nil {
				t.Fatal(err)
			}
			err := tc.build(root, st, dim, source)
			if err != nil {
				t.Fatalf("expected builtin fallback for %s, got %v", tc.remove, err)
			}
		})
	}
}

func promptFixture(t *testing.T) (string, Study, Dimension, Source) {
	t.Helper()
	root := t.TempDir()
	for _, dir := range []string{"prompts", "templates", "studies/demo/sources/repo", "studies/demo/reports/source", "studies/demo/reports/final", "studies/demo/dimensions"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFileContent(t, root, "# Base Prompt\n", "prompts", "base.md")
	writeFileContent(t, root, "# Synthesis Prompt\n", "prompts", "synthesize.md")
	writeFileContent(t, root, "# Repository Analysis\n", "templates", "repo-analysis.md")
	writeFileContent(t, root, "# Report\n", "templates", "report.md")
	st := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	writeFileContent(t, st.Path, "# Dimension\n", "dimensions", "01-structure.md")
	writeFileContent(t, st.Path, "---\napplicable_dimensions: [1]\n---\n# Document Body\n", "sources", "doc.md")
	writeFileContent(t, st.Path, "---\napplicable_dimensions: [2]\n---\n# Other Body\n", "sources", "other.md")
	dim := Dimension{Number: "01", Slug: "structure", File: "01-structure.md", Path: filepath.Join(st.Path, "dimensions", "01-structure.md")}
	source := Source{Name: "repo", Kind: SourceKindDirectory, Path: filepath.Join(st.Path, "sources", "repo")}
	return root, st, dim, source
}

func assertManifestEqual(t *testing.T, got, want PromptManifest) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("manifest = %s, want %s", gotJSON, wantJSON)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

func assertNotContains(t *testing.T, got, unwanted string) {
	t.Helper()
	if strings.Contains(got, unwanted) {
		t.Fatalf("expected %q not to contain %q", got, unwanted)
	}
}
