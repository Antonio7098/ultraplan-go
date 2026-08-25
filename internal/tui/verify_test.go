package tui

import (
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func TestSprintVerificationActionsAndNarrowSummary(t *testing.T) {
	data := fixtureDashboard()
	data.Sprints[0].Assessment = "pass_with_findings"
	data.Sprints[0].NextAction = "resolve issue ISSUE-1"
	data.Sprints[0].Review = app.ReviewSummary{Available: true, Status: "completed", Verdict: "pass_with_findings", Artifact: "projects/alpha/sprints/01/review.md"}
	data.Sprints[0].Smoke = app.SmokeSummary{Available: true, Status: "completed", Verdict: "pass_with_open_issues", RunID: "run-1", Artifact: "projects/alpha/sprints/01/smoke.md", Issues: []sprint.SmokeIssue{{ID: "ISSUE-1", Status: "open", Path: "issues/ISSUE-1.md"}}}
	m := Model{Data: data, Routes: []Route{{Kind: RouteSprint, Project: "alpha", Sprint: "01"}}}
	items := m.navItems()
	labels := ""
	for _, item := range items {
		labels += item.Label + "\n"
	}
	for _, want := range []string{"Verify to Conformance Review", "Verify to Smoke", "Flow to smoke", "Diagnostic Override"} {
		if !strings.Contains(labels, want) {
			t.Fatalf("actions missing %q: %s", want, labels)
		}
	}
	var b strings.Builder
	renderRouteSummary(&b, m)
	for _, want := range []string{"pass_with_findings", "run-1", "ISSUE-1", "resolve issue ISSUE-1"} {
		if !strings.Contains(b.String(), want) {
			t.Fatalf("summary missing %q: %s", want, b.String())
		}
	}
}
