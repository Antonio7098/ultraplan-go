package sprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const ApprovedExecuteTargetPath = "/home/antonioborgerees/coding/ultraplan/ultraplan-go"

func TestResolveExecuteTarget(t *testing.T) {
	target, findings := ResolveExecuteTarget("/workspace", testProjectIndex())
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	if target.Path != ApprovedExecuteTargetPath || target.Source != "project-index.md" {
		t.Fatalf("target = %+v", target)
	}
}

func TestResolveExecuteTargetSupportsWorkspaceRelativePaths(t *testing.T) {
	workspaceRoot := t.TempDir()
	targetRoot := filepath.Join(filepath.Dir(workspaceRoot), "project-source")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(workspaceRoot, targetRoot)
	if err != nil {
		t.Fatal(err)
	}
	target, findings := ResolveExecuteTarget(workspaceRoot, "- **Target Implementation Directory:** `"+relative+"`\n")
	if len(findings) != 0 || target.Path != targetRoot || target.Source != "project-index.md" {
		t.Fatalf("target=%+v findings=%+v", target, findings)
	}
}

func TestResolveExecuteTargetRejectsMissingAndUnavailableTargets(t *testing.T) {
	cases := map[string]string{
		"missing":     "# Project Index\n",
		"unavailable": "- **Target Implementation Directory:** ../missing-project\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, findings := ResolveExecuteTarget(t.TempDir(), content); len(findings) == 0 {
				t.Fatalf("expected findings")
			}
		})
	}
}

func TestValidateExecuteWorkdirContainment(t *testing.T) {
	target := ExecuteTargetRef{Path: ApprovedExecuteTargetPath, Source: "project-index.md"}
	if err := ValidateExecuteWorkdir(target, ApprovedExecuteTargetPath); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecuteWorkdir(target, ApprovedExecuteTargetPath+"/internal/sprint"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExecuteWorkdir(target, "/home/antonioborgerees/coding"); err == nil {
		t.Fatalf("expected escape rejection")
	}
	if err := ValidateExecuteWorkdir(target, "../ultraplan-go"); err == nil {
		t.Fatalf("expected relative rejection")
	}
}

func TestExecuteSafetyInstructionsExcludeDeferredBehavior(t *testing.T) {
	text := strings.Join(ExecuteSafetyInstructions(ExecuteTargetRef{Path: ApprovedExecuteTargetPath}), "\n")
	for _, want := range []string{"approved target", "smoke.md", "review.md", "issues.md", "Git mutation", "hosted/browser"} {
		if !strings.Contains(text, want) {
			t.Fatalf("instructions missing %q: %s", want, text)
		}
	}
}
