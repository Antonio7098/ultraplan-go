package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func TestQAViewUsesVerdictNeutralTextAtNarrowWidth(t *testing.T) {
	data := fixtureDashboard()
	fixture, err := os.ReadFile(filepath.Join("..", "testdata", "qa-canonical-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(fixture, &data.Sprints[0].QA); err != nil {
		t.Fatal(err)
	}
	data.Sprints[0].QA.ConformanceReviewVerdict = "fail"
	model := Model{Data: data, Routes: []Route{{Kind: RouteSprintQA, Project: data.Sprints[0].Project, Sprint: data.Sprints[0].Slug}}}
	var view strings.Builder
	renderRouteSummary(&view, model)
	for _, want := range []string{"Read-only QA completed", "Conformance Review: status=completed verdict=fail", "Coverage: 2/2", "Inspect retained outcomes."} {
		if !strings.Contains(view.String(), want) {
			t.Fatalf("QA view missing %q: %s", want, view.String())
		}
	}
	if strings.Contains(view.String(), "QA passed") {
		t.Fatalf("QA completion was promoted to a verdict: %s", view.String())
	}
}

func TestQAViewRendersFocusedStateAndExposesBoundedOperations(t *testing.T) {
	data := fixtureDashboard()
	qa := &data.Sprints[0].QA
	qa.Phase = "interrupted"
	qa.Fresh = true
	qa.RunID = "run_qa_1"
	qa.ChangedPaths, qa.CoveredPaths = 3, 3
	qa.CompletedShards, qa.TotalShards = 1, 2
	qa.Cancellation = app.QACancellationSummary{Requested: true, Reason: "operator requested stop"}
	qa.Blocker = &app.QABlockerSummary{Category: "qa.persistence_failure", Scope: "attempt", Summary: "state publication stopped", NextAction: "Recover QA state."}
	qa.NextAction = "Recover QA state."
	qa.Shards = []app.QAShardSummary{{ID: "qa-v1-shard-a", Kind: "primary", Title: "API boundary", Phase: "blocked", TheoryCount: 1, Theories: []app.QATheorySummary{{ID: "qa-v1-theory-a", Claim: "request loses correlation", Basis: "error branch", Outcome: "confirmed", OutcomeReason: "fixture evidence"}}}}
	route := Route{Kind: RouteSprintQATheory, Project: data.Sprints[0].Project, Sprint: data.Sprints[0].Slug, Shard: "qa-v1-shard-a", Theory: "qa-v1-theory-a"}
	model := Model{Data: data, Routes: []Route{route}}
	var view strings.Builder
	renderRouteSummary(&view, model)
	for _, want := range []string{"interrupted", "Cancellation requested", "state publication stopped", "Focused shard: API boundary", "Focused theory: qa-v1-theory-a", "Outcome: confirmed", "Recover QA state."} {
		if !strings.Contains(view.String(), want) {
			t.Fatalf("focused QA view missing %q: %s", want, view.String())
		}
	}

	model.Routes = []Route{{Kind: RouteSprintQA, Project: data.Sprints[0].Project, Sprint: data.Sprints[0].Slug}}
	items := model.navItems()
	labels := ""
	for _, item := range items {
		labels += item.Label + "\n"
	}
	for _, want := range []string{"QA Status", "QA Dry Run", "Start QA [RUNTIME]", "Resume QA [RUNTIME]", "Recover QA", "View QA durable run", "API boundary"} {
		if !strings.Contains(labels, want) {
			t.Fatalf("QA navigation missing %q: %s", want, labels)
		}
	}
}

func TestRepairViewKeepsSemanticOutcomeSeparateAndOffersOneGuardedStart(t *testing.T) {
	data := fixtureDashboard()
	data.Sprints[0].Repair = app.RepairStatusResult{
		SchemaVersion: 1, Project: data.Sprints[0].Project, Sprint: data.Sprints[0].Slug,
		Phase: "prepared", Fresh: true, Mode: "manual", RepairRunID: "repair-v1-run-aaaaaaaaaaaaaaaaaaaaaaaa", RunLifecycle: "terminal",
		Packet:     &app.RepairPacketSummary{Digest: "sha256:packet", IssueID: "qa-v1-issue-current", IssueTitle: "bounded failure", Target: app.QATargetIdentitySummary{Fingerprint: "target"}, CheckCount: 7, Budgets: app.RepairBudgetSummary{MaxMutationCycles: 1, MaxFiles: 8, MaxBytes: 1024, WallTime: "45m0s"}},
		NextAction: "Review and confirm the frozen packet.",
	}
	model := Model{Data: data, Routes: []Route{{Kind: RouteSprintRepair, Project: data.Sprints[0].Project, Sprint: data.Sprints[0].Slug}}}
	var view strings.Builder
	renderRouteSummary(&view, model)
	for _, want := range []string{"Bounded repair", "Fresh authority: true", "Issue: qa-v1-issue-current", "Limits: 1 apply", "Automatic mode: unavailable", "Review and confirm"} {
		if !strings.Contains(view.String(), want) {
			t.Fatalf("repair view missing %q: %s", want, view.String())
		}
	}
	items := model.navItems()
	guarded := 0
	for _, item := range items {
		if item.Operation != nil && item.Operation.Kind == app.OperationRepairStart {
			guarded++
			if item.Operation.RepairRunID != data.Sprints[0].Repair.RepairRunID || item.Operation.RepairConfirmer == "" {
				t.Fatalf("guarded repair request=%+v", item.Operation)
			}
		}
	}
	if guarded != 1 {
		t.Fatalf("guarded repair starts=%d items=%+v", guarded, items)
	}
	data.Sprints[0].Repair.Confirmation = &app.RepairConfirmSummary{Digest: "confirmation", Confirmer: "operator"}
	model.Data = data
	for _, item := range model.navItems() {
		if item.Operation != nil && item.Operation.Kind == app.OperationRepairStart {
			t.Fatal("confirmed packet still offered another start confirmation")
		}
	}
}
