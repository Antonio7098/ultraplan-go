package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStudyListUsesWorkspaceAndSortsStudies(t *testing.T) {
	dir := initializedWorkspace(t)
	mkdirAll(t, dir, "studies", "zeta")
	mkdirAll(t, dir, "studies", "alpha")
	mkdirAll(t, dir, "studies", ".hidden")
	writeFixtureFile(t, dir, "studies", "not-a-study")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "list"})
	if status != ExitOK {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Workspace: "+dir)
	assertInOrder(t, stdout, "  alpha\n", "  zeta\n")
	assertNotContains(t, stdout, ".hidden")
	assertNotContains(t, stdout, "not-a-study")
}

func TestStudyListEmpty(t *testing.T) {
	dir := initializedWorkspace(t)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "list"})
	if status != ExitOK {
		t.Fatalf("status = %d, stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Studies:\n  (none)")
}

func TestStudyDetailListsSourcesDimensionsAndKind(t *testing.T) {
	dir := initializedWorkspace(t)
	studyRoot := filepath.Join(dir, "studies", "platform")
	mkdirAll(t, studyRoot, "sources", "zeta", "nested")
	mkdirAll(t, studyRoot, "sources", "alpha")
	writeFixtureFile(t, studyRoot, "sources", "document.md")
	writeFixtureFile(t, studyRoot, "sources", "zeta", "nested", "ignored.txt")
	writeFixtureFile(t, studyRoot, "dimensions", "02-runtime.md")
	writeFixtureFile(t, studyRoot, "dimensions", "01-structure.md")
	writeFixtureFile(t, studyRoot, "dimensions", "notes.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "plat", "list"})
	if status != ExitOK {
		t.Fatalf("status = %d, stderr = %q stdout = %q", status, stderr, stdout)
	}
	assertContains(t, stdout, "Study: platform")
	assertInOrder(t, stdout, "  alpha directory\n", "  zeta directory\n")
	assertInOrder(t, stdout, "  01 structure 01-structure.md\n", "  02 runtime 02-runtime.md\n")
	assertNotContains(t, stdout, "document.md")
	assertNotContains(t, stdout, "ignored.txt")
}

func TestStudyDetailMissingAndAmbiguousStudyRefsAreActionable(t *testing.T) {
	dir := initializedWorkspace(t)
	mkdirAll(t, dir, "studies", "api")
	mkdirAll(t, dir, "studies", "api-v2")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "missing", "list"})
	if status != ExitValidation {
		t.Fatalf("missing status = %d, stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `study reference "missing" not found`)
	assertContains(t, stderr, "available: api, api-v2")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "ap", "list"})
	if status != ExitValidation {
		t.Fatalf("ambiguous status = %d, stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `ambiguous study reference "ap"`)
	assertContains(t, stderr, "api, api-v2")
}

func mkdirAll(t *testing.T, base string, rel ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(append([]string{base}, rel...)...), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFixtureFile(t *testing.T, base string, rel ...string) {
	t.Helper()
	path := filepath.Join(append([]string{base}, rel...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertInOrder(t *testing.T, got string, first string, second string) {
	t.Helper()
	firstIndex := stringsIndex(got, first)
	secondIndex := stringsIndex(got, second)
	if firstIndex < 0 || secondIndex < 0 || firstIndex >= secondIndex {
		t.Fatalf("expected %q before %q in:\n%s", first, second, got)
	}
}

func stringsIndex(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
