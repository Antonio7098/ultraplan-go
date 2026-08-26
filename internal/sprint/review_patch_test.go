package sprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildReviewPatchUsesRecordedBaselineAndIncludesUntrackedFiles(t *testing.T) {
	source := gitFixture(t)
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-review")
	target, findings := NewService(root).WithClock(func() time.Time { return time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC) }).resolveSprintTarget(sp, projectIndexForTarget(source), true)
	if len(findings) != 0 {
		t.Fatalf("create sprint worktree: %+v", findings)
	}
	if err := os.WriteFile(filepath.Join(target.Path, "README.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target.Path, "new.txt"), []byte("new evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, available, err := buildReviewPatch(sp, target.Path, []string{"README.md", "new.txt"})
	if err != nil || !available {
		t.Fatalf("patch available=%v err=%v", available, err)
	}
	for _, want := range []string{"README.md", "-baseline", "+changed", "new.txt", "+new evidence"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("patch missing %q:\n%s", want, patch)
		}
	}
}

func TestReviewerPromptInjectsPatchAndHandoffButNotChangedFileContents(t *testing.T) {
	manifest := ReviewManifest{
		PromptTemplate: "# Review",
		Target:         "/target",
		Fingerprint:    strings.Repeat("a", 64),
		ChangedPaths:   []string{"internal/changed.go"},
		Contents: map[string]string{
			"contracts/review.md":         "coverage contract marker\n",
			"projects/proj/.handoff.json": "execution handoff marker\n",
			reviewPatchPath:               "implementation diff marker\n",
			"target/internal/changed.go":  "full changed file marker\n",
		},
	}
	manifest.Inputs = []ReviewInput{
		reviewInput("contract-review", "contract", "review", "contracts/review.md", manifest.Contents["contracts/review.md"]),
		reviewInput("execution-handoff", "execution", "handoff", "projects/proj/.handoff.json", manifest.Contents["projects/proj/.handoff.json"]),
		reviewInput("implementation-diff", "implementation-diff", "diff", reviewPatchPath, manifest.Contents[reviewPatchPath]),
		reviewInput("target-internal-changed-go", "target", "changed.go", "target/internal/changed.go", manifest.Contents["target/internal/changed.go"]),
	}
	manifest.Coverage = []ReviewInput{manifest.Inputs[0]}
	prompt := renderReviewerPrompt(manifest, manifest.Coverage[0])
	for _, want := range []string{"coverage contract marker", "execution handoff marker", "implementation diff marker", "raw run state is intentionally not supplied"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "full changed file marker") {
		t.Fatalf("prompt embedded the complete changed file:\n%s", prompt)
	}
}
