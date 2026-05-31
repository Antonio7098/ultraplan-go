package study

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInitDryRunPlansDeterministicArtifactsAndNoMutation(t *testing.T) {
	root := t.TempDir()
	input := writeInitYAML(t, root, validInitYAML())

	result, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.StudyName != "go-cli-study" {
		t.Fatalf("StudyName = %q", result.StudyName)
	}
	assertHasRel(t, root, result.Directories, "studies/go-cli-study/dimensions")
	assertHasRel(t, root, result.Files, "studies/go-cli-study/study-init.yml")
	assertHasRel(t, root, result.Files, "studies/go-cli-study/dimensions/01-project-structure.md")
	if len(result.CloneActions) != 1 {
		t.Fatalf("CloneActions = %+v", result.CloneActions)
	}
	if _, err := os.Stat(filepath.Join(root, "studies", "go-cli-study")); !os.IsNotExist(err) {
		t.Fatalf("dry-run mutated filesystem: %v", err)
	}
}

func TestInitCreatesArtifactsAndSkipsClones(t *testing.T) {
	root := t.TempDir()
	input := writeInitYAML(t, root, validInitYAML())
	runner := &recordingCloneRunner{}

	result, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, NoClone: true, CloneRunner: runner})
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("clone calls = %+v, want none", runner.calls)
	}
	if len(result.SkippedClones) != 1 {
		t.Fatalf("SkippedClones = %+v", result.SkippedClones)
	}
	assertFileContains(t, root, "studies/go-cli-study/study-init.yml", `number: "01"`)
	assertFileContains(t, root, "studies/go-cli-study/README.md", "ultraplan study go-cli-study list")
	assertFileContains(t, root, "studies/go-cli-study/dimensions/01-project-structure.md", "## Citations")
}

func TestInitCloneRunnerArgsAndPartialFailure(t *testing.T) {
	root := t.TempDir()
	input := writeInitYAML(t, root, validInitYAML())
	runner := &recordingCloneRunner{err: errors.New("network disabled")}

	result, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, CloneRunner: runner})
	if !errors.Is(err, ErrInitPartial) {
		t.Fatalf("err = %v, want ErrInitPartial", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %+v", runner.calls)
	}
	if runner.calls[0].url != "https://github.com/org/example" {
		t.Fatalf("url = %q", runner.calls[0].url)
	}
	if filepath.Base(runner.calls[0].dest) != "example" {
		t.Fatalf("dest = %q", runner.calls[0].dest)
	}
	assertFileContains(t, root, "studies/go-cli-study/README.md", "Comparative architecture study")
	if len(result.CloneFailures) != 1 {
		t.Fatalf("CloneFailures = %+v", result.CloneFailures)
	}
}

func TestInitValidationFailuresAreActionable(t *testing.T) {
	for _, tc := range []struct {
		name string
		yaml string
		want string
	}{
		{name: "invalid yaml", yaml: "name: [", want: "parse YAML"},
		{name: "count shortage", yaml: replace(validInitYAML(), "count: 1\n  items:", "count: 2\n  items:"), want: "assisted completion is deferred"},
		{name: "duplicate source", yaml: replace(validInitYAML(), "name: guide", "name: example"), want: "duplicates source"},
		{name: "duplicate dimension slug", yaml: validInitYAML() + dimensionItemYAML("02", "Project Structure"), want: "duplicates dimension slug"},
		{name: "unsafe source path", yaml: replace(validInitYAML(), "path: sources/guide.md", "path: ../guide.md"), want: "safe relative path"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			input := writeInitYAML(t, root, tc.yaml)
			_, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, DryRun: true})
			if !errors.Is(err, ErrInitValidation) {
				t.Fatalf("err = %v, want ErrInitValidation", err)
			}
			if !contains(err.Error(), tc.want) {
				t.Fatalf("err = %q, want %q", err.Error(), tc.want)
			}
		})
	}
}

func TestInitValidationFailuresRemainStructured(t *testing.T) {
	root := t.TempDir()
	input := writeInitYAML(t, root, "name: bad name\n")

	_, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, DryRun: true})
	if !errors.Is(err, ErrInitValidation) {
		t.Fatalf("err = %v, want ErrInitValidation", err)
	}
	var validationErr InitValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %T, want InitValidationError", err)
	}
	if len(validationErr.Problems) < 2 {
		t.Fatalf("Problems = %+v, want multiple distinct problems", validationErr.Problems)
	}
	for _, problem := range validationErr.Problems {
		if problem.Code == "" || problem.Message == "" {
			t.Fatalf("problem missing code or message: %+v", problem)
		}
	}
}

func TestRedactGitOutputRemovesCredentialsAndBoundsOutput(t *testing.T) {
	secret := "https://user:token@example.com/repo.git"
	output := secret + "\n" + string(make([]byte, 5000))

	redacted := redactGitOutput(output)
	if contains(redacted, "user:token") {
		t.Fatalf("redacted output leaked credentials: %q", redacted)
	}
	if !contains(redacted, "[redacted]@example.com") {
		t.Fatalf("redacted output = %q, want redacted URL", redacted)
	}
	if len(redacted) > 4120 {
		t.Fatalf("redacted output length = %d, want bounded output", len(redacted))
	}
}

func TestInitOutputPathSafetyOverwriteAndForce(t *testing.T) {
	root := t.TempDir()
	input := writeInitYAML(t, root, validInitYAML())
	_, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, OutputDir: "../outside", DryRun: true})
	if !errors.Is(err, ErrInitValidation) {
		t.Fatalf("outside err = %v, want validation", err)
	}

	if _, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, NoClone: true}); err != nil {
		t.Fatal(err)
	}
	_, err = Init(InitRequest{WorkspaceRoot: root, InputPath: input, NoClone: true})
	if !errors.Is(err, ErrInitOverwrite) {
		t.Fatalf("overwrite err = %v, want ErrInitOverwrite", err)
	}
	unknown := filepath.Join(root, "studies", "go-cli-study", "notes.txt")
	if err := os.WriteFile(unknown, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(InitRequest{WorkspaceRoot: root, InputPath: input, NoClone: true, Force: true}); err != nil {
		t.Fatal(err)
	}
	assertFileContains(t, root, "studies/go-cli-study/notes.txt", "keep")
}

type cloneCall struct{ url, dest string }

type recordingCloneRunner struct {
	calls []cloneCall
	err   error
}

func (r *recordingCloneRunner) Clone(url, dest string) error {
	r.calls = append(r.calls, cloneCall{url: url, dest: dest})
	return r.err
}

func writeInitYAML(t *testing.T, root, content string) string {
	t.Helper()
	path := filepath.Join(root, "study-init.yml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func validInitYAML() string {
	return `name: go-cli-study
description: Comparative architecture study
repos:
  count: 2
  items:
    - name: example
      url: https://github.com/org/example
      description: Example repository
    - name: guide
      path: sources/guide.md
      description: Guide document
dimensions:
  count: 1
  items:
` + dimensionItemYAML("1", "Project Structure")
}

func dimensionItemYAML(number, name string) string {
	return `    - number: "` + number + `"
      name: ` + name + `
      title: Project Structure
      description: Boundaries and package layout
      purpose: What this dimension analyzes
      steps:
        - Inspect module layout
      citations:
        - Source files implementing boundaries
      questions:
        - How are responsibilities separated?
`
}

func assertHasRel(t *testing.T, root string, paths []string, want string) {
	t.Helper()
	for _, path := range paths {
		if filepath.ToSlash(path[len(root)+1:]) == want {
			return
		}
	}
	t.Fatalf("missing %s in %v", want, paths)
}

func assertFileContains(t *testing.T, root, rel, want string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(data), want) {
		t.Fatalf("%s missing %q:\n%s", rel, want, data)
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func replace(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}
