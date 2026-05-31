package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestStudyInitHelpAndUsage(t *testing.T) {
	dir := initializedWorkspace(t)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "init", "--help"})
	if status != ExitOK {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	for _, want := range []string{"ultraplan study init <study-init.yml>", "--dry-run", "--force", "--no-clone", "--output"} {
		assertContains(t, stdout, want)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "init"})
	if status != ExitUsage {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "requires <study-init.yml>")
}

func TestStudyInitDryRunOutputAndNoMutation(t *testing.T) {
	dir := initializedWorkspace(t)
	input := writeAppInitYAML(t, dir)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "init", input, "--dry-run"})
	if status != ExitOK {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Would initialize study: cli-study")
	assertContains(t, stdout, "studies/cli-study/dimensions")
	assertContains(t, stdout, "studies/cli-study/dimensions/01-command-architecture.md")
	assertContains(t, stdout, "repo -> studies/cli-study/sources/repo")
	if _, err := os.Stat(filepath.Join(dir, "studies", "cli-study")); !os.IsNotExist(err) {
		t.Fatalf("dry-run created study: %v", err)
	}
}

func TestStudyInitNoCloneOutputWorkspaceOutputAndForce(t *testing.T) {
	dir := initializedWorkspace(t)
	input := writeAppInitYAML(t, dir)
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "init", input, "--no-clone", "--output", "custom/cli-study"})
	if status != ExitOK {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Output: custom/cli-study")
	assertContains(t, stdout, "Skipped clone actions due to --no-clone")
	if _, err := os.Stat(filepath.Join(dir, "custom", "cli-study", "README.md")); err != nil {
		t.Fatalf("README missing: %v", err)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "init", input, "--no-clone", "--output", "custom/cli-study"})
	if status != ExitValidation {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "use --force")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "init", input, "--no-clone", "--force", "--output", "custom/cli-study"})
	if status != ExitOK {
		t.Fatalf("force status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
}

func TestStudyInitValidationAndUnsafeOutput(t *testing.T) {
	dir := initializedWorkspace(t)
	input := filepath.Join(dir, "bad.yml")
	if err := os.WriteFile(input, []byte("name: bad\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "init", input, "--dry-run"})
	if status != ExitValidation {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "description is required")

	input = writeAppInitYAML(t, dir)
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "study", "init", input, "--dry-run", "--output", "../outside"})
	if status != ExitValidation {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "escapes workspace")
}

func TestStudyInitClonePartialFailureExitMapping(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable script is POSIX-specific")
	}
	dir := initializedWorkspace(t)
	input := writeAppInitYAML(t, dir)
	bin := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'fake clone failure' >&2\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "study", "init", input})
	if status != ExitPartial {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Initialized study: cli-study")
	assertContains(t, stderr, "clone failed for repo")
	assertContains(t, stderr, "provider.git.clone_failed")
	assertContains(t, stderr, "study init partial")
	if _, err := os.Stat(filepath.Join(dir, "studies", "cli-study", "README.md")); err != nil {
		t.Fatalf("README missing after partial clone: %v", err)
	}
}

func TestStudyInitClonePartialFailureRedactsGitOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable script is POSIX-specific")
	}
	dir := initializedWorkspace(t)
	input := writeAppInitYAML(t, dir)
	bin := filepath.Join(t.TempDir(), "git")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf 'fatal: could not read https://user:token@example.com/repo.git' >&2\nexit 42\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, stderr, status := runForTest([]string{"--workspace", dir, "study", "init", input})
	if status != ExitPartial {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stderr, "[redacted]@example.com")
	assertNotContains(t, stderr, "user:token")
}

func writeAppInitYAML(t *testing.T, dir string) string {
	t.Helper()
	input := filepath.Join(dir, "study-init.yml")
	content := `name: cli-study
description: CLI study
repos:
  count: 1
  items:
    - name: repo
      url: https://github.com/org/repo
      description: Repo source
dimensions:
  count: 1
  items:
    - number: "1"
      name: Command Architecture
      title: Command Architecture
      description: Command structure
      purpose: Inspect command wiring
      steps:
        - Read command files
      citations:
        - Command source files
      questions:
        - Are handlers thin?
`
	if err := os.WriteFile(input, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return input
}
