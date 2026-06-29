package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStudyPromptAnalysisPreviewStdoutAndOutputFile(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "analysis", "01", "repo"})
	if status != ExitOK {
		t.Fatalf("status = %d, stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "--- manifest ---")
	assertContains(t, stdout, `"kind": "directory_analysis"`)
	assertContains(t, stdout, "--- prompt ---")
	assertContains(t, stdout, "Inspect only the selected source directory")
	assertNotContains(t, stdout, "agentwrap")
	assertNotContains(t, stdout, "OpenCode")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "analysis", "01", "repo", "--output", "previews/repo.prompt.md"})
	if status != ExitOK {
		t.Fatalf("output status = %d, stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Wrote prompt preview: previews/repo.prompt.md")
	content, err := os.ReadFile(filepath.Join(dir, "previews", "repo.prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(content), filepath.ToSlash(filepath.Join("studies", filepath.Base(studyRoot), "reports", "source", "repo-01-structure.md")))
}

func TestStudyPromptSynthesisPreview(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)
	writeFixtureFileContent(t, studyRoot, "# Repo report\n", "reports", "source", "repo-01-structure.md")
	writeFixtureFileContent(t, studyRoot, "# Doc report\n", "reports", "source", "doc.md-01-structure.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "synthesis", "structure"})
	if status != ExitOK {
		t.Fatalf("status = %d, stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, `"kind": "synthesis"`)
	assertContains(t, stdout, `"source": "doc.md"`)
	assertContains(t, stdout, `"source": "repo"`)
	assertContains(t, stdout, "studies/demo/reports/final/report.md")
}

func TestStudyPromptFailuresAreActionable(t *testing.T) {
	dir, studyRoot := promptCommandFixture(t)

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "missing", "prompt", "analysis", "01", "repo"})
	if status != ExitValidation {
		t.Fatalf("missing study status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, `study reference "missing" not found`)

	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "analysis", "missing", "repo"})
	if status != ExitValidation {
		t.Fatalf("missing dimension status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, `dimension reference "missing" not found`)

	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "analysis", "01", "missing"})
	if status != ExitValidation {
		t.Fatalf("missing source status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, `source reference "missing" not found`)

	mkdirAll(t, filepath.Join(dir, "studies"), "demo-v2")
	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "dem", "prompt", "analysis", "01", "repo"})
	if status != ExitValidation {
		t.Fatalf("ambiguous study status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, `ambiguous study reference "dem"`)

	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "analysis", "01", "other.md"})
	if status != ExitValidation {
		t.Fatalf("inapplicable status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "does not apply to dimension")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "analysis", "01", "repo"})
	if status != ExitOK {
		t.Fatalf("builtin prompt fallback status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "builtin:prompts/base.md")

	writeFixtureFileContent(t, dir, "# Base Prompt\n", "prompts", "base.md")
	if err := os.Remove(filepath.Join(studyRoot, "sources", "doc.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(studyRoot, "sources", "missing-target.md"), filepath.Join(studyRoot, "sources", "doc.md")); err != nil {
		t.Fatal(err)
	}
	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "analysis", "01", "doc.md"})
	if status != ExitWorkspace {
		t.Fatalf("missing doc status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "doc.md")
	if err := os.Remove(filepath.Join(studyRoot, "sources", "doc.md")); err != nil {
		t.Fatal(err)
	}
	writeFixtureFileContent(t, studyRoot, "---\napplicable_dimensions: [1]\n---\n# Doc\n", "sources", "doc.md")
	writeFixtureFileContent(t, studyRoot, "# Doc report\n", "reports", "source", "doc.md-01-structure.md")

	_, stderr, status = runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "synthesis", "01"})
	if status != ExitWorkspace {
		t.Fatalf("missing report status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "repo-01-structure.md")
}

func TestStudyPromptHelpDescribesNoRuntimeBoundary(t *testing.T) {
	dir, _ := promptCommandFixture(t)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "demo", "prompt", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "does not execute runtime analysis")
	assertContains(t, stdout, "subprocesses")
}

func promptCommandFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "demo")
	mkdirAll(t, studyRoot, "sources", "repo")
	mkdirAll(t, studyRoot, "reports", "source")
	mkdirAll(t, studyRoot, "reports", "final")
	writeFixtureFileContent(t, studyRoot, "# Structure\n", "dimensions", "01-structure.md")
	writeFixtureFileContent(t, studyRoot, "---\napplicable_dimensions: [1]\n---\n# Doc\n", "sources", "doc.md")
	writeFixtureFileContent(t, studyRoot, "---\napplicable_dimensions: [2]\n---\n# Other\n", "sources", "other.md")
	return dir, studyRoot
}
