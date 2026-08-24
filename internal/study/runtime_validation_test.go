package study

import (
	"strings"
	"testing"

	"github.com/Antonio7098/agentwrap"
)

func TestStudyValidationRepairsContinueTheSameSession(t *testing.T) {
	spec := studyReportValidationSpec(TaskKindAnalysis, Study{}, Source{}, Dimension{}, "/tmp/report.md")
	if spec.Repair.MaxAttempts != 2 {
		t.Fatalf("repair attempts = %d, want 2", spec.Repair.MaxAttempts)
	}
	if spec.Repair.SessionAction != agentwrap.SessionActionContinue {
		t.Fatalf("session action = %q, want continue", spec.Repair.SessionAction)
	}
	if spec.Repair.OverrideRequest != nil {
		t.Fatal("repair request override could replace the original session")
	}
	if !spec.Repair.AllowFreshSessionFallback || !spec.Repair.FreshSessionFallbackOnError {
		t.Fatal("fresh-session fallback should remain available when continuation fails")
	}
}

func TestStudyRepairPromptIncludesValidationFailure(t *testing.T) {
	prompt := buildStudyReportRepairPrompt("/tmp/report.md", []agentwrap.ValidationFailure{{Observed: "rating.parse: observed missing rating"}})
	for _, want := range []string{"/tmp/report.md", "rating.parse: observed missing rating", "Repair only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("repair prompt missing %q: %s", want, prompt)
		}
	}
}
