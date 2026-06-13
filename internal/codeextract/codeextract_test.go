package codeextract

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractResolvesRangesListsBasenamesAndIgnoredDirs(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "sources", "repo")
	mustWrite(t, filepath.Join(sourceRoot, "sub", "target.go"), "a\nb\nc\nd\n")
	mustWrite(t, filepath.Join(sourceRoot, "node_modules", "target.go"), "ignored\n")
	report := filepath.Join(root, "reports", "report.md")
	mustWrite(t, report, "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nRefs: `sub/target.go:1–2` and `target.go:4`.\n")

	result, err := Extract(Request{WorkspaceRoot: root, Reports: []string{report}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusOK {
		t.Fatalf("status = %s unresolved = %+v", result.Status, result.Unresolved)
	}
	refs := result.Reports[0].References
	if len(refs) != 2 {
		t.Fatalf("refs = %+v", refs)
	}
	if refs[0].LineSpec != "1-2" || refs[0].Snippet[0].Number != 1 || refs[0].Snippet[1].Text != "b" {
		t.Fatalf("range ref = %+v", refs[0])
	}
	if refs[1].Snippet[0].Number != 4 || refs[1].Snippet[0].Text != "d" {
		t.Fatalf("basename ref = %+v", refs[1])
	}
	var out bytes.Buffer
	if err := RenderText(&out, root, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Unresolved: 0")) {
		t.Fatalf("text output:\n%s", out.String())
	}
}

func TestExtractReportsPathEscapeOutOfRangeAndMalformedSpecs(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "sources", "repo")
	mustWrite(t, filepath.Join(sourceRoot, "main.go"), "one\n")
	report := filepath.Join(root, "report.md")
	mustWrite(t, report, "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nRefs: `../secret.go:1`, `main.go:9`, and `main.go:2-1`.\n")

	result, err := Extract(Request{WorkspaceRoot: root, Reports: []string{report}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusPartial {
		t.Fatalf("status = %s unresolved = %+v", result.Status, result.Unresolved)
	}
	if len(result.Unresolved) != 3 {
		t.Fatalf("unresolved = %+v", result.Unresolved)
	}
}

func TestExtractReportWithSourceTableAndNoSupportedCitations(t *testing.T) {
	root := t.TempDir()
	sourceRoot := filepath.Join(root, "sources", "repo")
	mustWrite(t, filepath.Join(sourceRoot, "main.go"), "package main\n")
	report := filepath.Join(root, "report.md")
	mustWrite(t, report, "| # | Source | Path |\n| 1 | repo | `sources/repo` |\n\nThis report has prose and [a markdown link](main.go#L1), but no supported inline code citation.\n")

	result, err := Extract(Request{WorkspaceRoot: root, Reports: []string{report}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusOK {
		t.Fatalf("status = %s unresolved = %+v", result.Status, result.Unresolved)
	}
	if len(result.Reports[0].References) != 0 {
		t.Fatalf("references = %+v", result.Reports[0].References)
	}
	var out bytes.Buffer
	if err := RenderText(&out, root, result); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out.Bytes(), []byte("References: 0")) {
		t.Fatalf("text output:\n%s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Unresolved: 0")) {
		t.Fatalf("text output:\n%s", out.String())
	}
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
