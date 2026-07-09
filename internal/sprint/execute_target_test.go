package sprint

import (
	"strings"
	"testing"
)

func TestResolveExecuteTarget(t *testing.T) {
	target, findings := ResolveExecuteTarget(testProjectIndex())
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	if target.Path != ApprovedExecuteTargetPath || target.Source != "project-index.md" {
		t.Fatalf("target = %+v", target)
	}
}

func TestResolveExecuteTargetRejectsMissingRelativeAndAlternateTargets(t *testing.T) {
	cases := map[string]string{
		"missing":   "# Project Index\n",
		"relative":  "- **Target Implementation Directory:** ../ultraplan-go\n",
		"alternate": "- **Target Implementation Directory:** /tmp/ultraplan-go\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, findings := ResolveExecuteTarget(content); len(findings) == 0 {
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
