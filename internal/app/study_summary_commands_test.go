package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStudySummaryCommandWritesCSVAndWarnings(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "demo")
	writeFixtureFileContent(t, studyRoot, "# Dim\n", "dimensions", "01-structure.md")
	mkdirAll(t, studyRoot, "sources", "repo")
	writeFixtureFileContent(t, studyRoot, "# Report\n\nRating: 9\n", "reports", "source", "01-structure", "repo.md")
	writeFixtureFileContent(t, studyRoot, "# Body\n", "sources", "missing.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "summary"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Summary: studies/demo/summary.csv")
	assertContains(t, stderr, "Warning: source=missing.md dimension=01-structure")
	assertContains(t, stderr, "missing report")
	content, err := os.ReadFile(filepath.Join(studyRoot, "summary.csv"))
	if err != nil {
		t.Fatal(err)
	}
	want := "source,01-structure,total\nrepo,9,9\nmissing.md,,0\n"
	if string(content) != want {
		t.Fatalf("summary:\n%s\nwant:\n%s", content, want)
	}
}

func TestStudySummaryCommandHelp(t *testing.T) {
	dir := initializedWorkspace(t)
	mkdirAll(t, dir, "studies", "demo")

	for _, flag := range []string{"--help", "-h"} {
		t.Run(flag, func(t *testing.T) {
			stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "summary", flag})
			if status != ExitOK {
				t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
			}
			assertContains(t, stdout, "ultraplan study <study> summary")
			assertContains(t, stdout, "Regenerates studies/<study>/summary.csv")
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
		})
	}
}
