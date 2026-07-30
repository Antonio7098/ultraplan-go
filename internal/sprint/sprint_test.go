package sprint

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/project"
)

func TestDomainStagesStatusesAndArtifactPaths(t *testing.T) {
	stages := PlanningStages()
	wantStages := []PlanningStage{StageRequirements, StageSprintIndex, StageTechnicalHandbook, StageAreaReasoning, StageReasoning, StagePlan}
	if strings.Join(stageStrings(stages), ",") != strings.Join(stageStrings(wantStages), ",") {
		t.Fatalf("stages = %v", stages)
	}
	if ValidStage("implementation") || ValidStage("review") {
		t.Fatalf("unsupported stage accepted")
	}
	for _, status := range []StageStatus{StatusMissing, StatusReady, StatusComplete, StatusFailed, StatusSkipped} {
		if !ValidStatus(status) {
			t.Fatalf("status %q rejected", status)
		}
	}
	if ValidStatus("running") {
		t.Fatalf("unsupported status accepted")
	}
	sp := Sprint{Project: "proj", Slug: "01-sprint"}
	if got := ArtifactRelPath(sp, StageAreaReasoning); got != "projects/proj/sprints/01-sprint/reasoning" {
		t.Fatalf("area path = %q", got)
	}
	if got := FlowStateRelPath(sp); got != "projects/proj/sprints/01-sprint/flow-state.json" {
		t.Fatalf("flow path = %q", got)
	}
}

func TestDiscoveryResolutionAndDerivation(t *testing.T) {
	root := workspaceFixture(t)
	p := project.Project{Name: "proj", Path: filepath.Join(root, "projects", "proj")}
	mkdirAll(t, p.Path, "sprints", "02-beta")
	mkdirAll(t, p.Path, "sprints", "01-alpha")
	mkdirAll(t, p.Path, "sprints", ".hidden")
	mkdirAll(t, p.Path, "sprints", "bad name")
	writeFile(t, p.Path, "sprints", "file")

	sprints, err := DiscoverSprints(root, p)
	if err != nil {
		t.Fatal(err)
	}
	if got := sprintNames(sprints); strings.Join(got, ",") != "01-alpha,02-beta" {
		t.Fatalf("sprints = %v", got)
	}
	if _, err := ResolveSprint(sprints, "../bad"); err == nil {
		t.Fatalf("expected invalid ref")
	}
	if got, err := ResolveSprint(sprints, "01"); err != nil || got.Slug != "01-alpha" {
		t.Fatalf("prefix resolve = %+v %v", got, err)
	}
	if _, err := ResolveSprint(sprints, "0"); err == nil {
		t.Fatalf("expected ambiguous ref")
	}

	sp := Sprint{Project: "proj", Slug: "01-alpha", Path: filepath.Join(p.Path, "sprints", "01-alpha")}
	snap := ArtifactSnapshot{Files: map[PlanningStage]bool{StageRequirements: true, StageSprintIndex: true}}
	derived := DeriveStages(sp, snap, nil)
	if derived[0].Status != StatusComplete || derived[1].Status != StatusComplete || derived[2].Status != StatusReady || derived[3].Status != StatusMissing {
		t.Fatalf("partial statuses = %+v", derived)
	}
	snap.NoReasoningSelected = true
	snap.Files[StageTechnicalHandbook] = true
	derived = DeriveStages(sp, snap, nil)
	if derived[3].Status != StatusSkipped || derived[4].Status != StatusReady {
		t.Fatalf("skip statuses = %+v", derived)
	}
	prior := []StageState{{Stage: StageReasoning, Status: StatusFailed, Path: ArtifactRelPath(sp, StageReasoning), Error: "runtime failed"}}
	derived = DeriveStages(sp, snap, prior)
	if derived[4].Status != StatusFailed || derived[4].Error != "runtime failed" {
		t.Fatalf("failed state not preserved: %+v", derived[4])
	}
	snap.Files[StageReasoning] = true
	derived = DeriveStages(sp, snap, prior)
	if derived[4].Status != StatusComplete || derived[4].Error != "" {
		t.Fatalf("stale failed state not cleared: %+v", derived[4])
	}
}

func TestFlowStateStrictLoadingAndAtomicWritePreservesPrior(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	now := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	state := NewFlowState(sp, completeStates(sp), now)
	if err := SaveFlowState(root, sp, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SchemaVersion != FlowStateSchemaVersion || len(loaded.Stages) != len(PlanningStages()) {
		t.Fatalf("loaded = %+v", loaded)
	}

	path, err := FlowStatePath(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	bad := state
	bad.Stages[0].Stage = "implementation"
	writeJSON(t, path, bad)
	if _, err := LoadFlowState(root, sp); !errors.Is(err, ErrFlowStateMalformed) {
		t.Fatalf("unsupported stage err = %v", err)
	}
	writeFileContent(t, filepath.Dir(path), string(original), filepath.Base(path))

	bad = state
	bad.Stages[0].Path = "../outside.md"
	if err := ValidateFlowState(root, sp, bad, path); !errors.Is(err, ErrFlowStateMalformed) {
		t.Fatalf("unsafe path err = %v", err)
	}
	err = saveFlowStateWithHooks(root, sp, state, atomicWriteHooks{BeforeRename: func(string) error {
		return errors.New("rename blocked")
	}})
	if err == nil {
		t.Fatalf("expected write failure")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("prior state was not preserved")
	}
}

func TestServiceStatusRefreshesMissingStateAndRejectsInvalidState(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFileContent(t, sp.Path, "# Requirements\n", "requirements.md")
	writeFileContent(t, sp.Path, "# Sprint Index\n\nNo reasoning templates selected.\n", "sprint-index.md")

	status, err := NewService(root).Status("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if status.Project != "proj" || status.Sprint != "01-alpha" || status.Stages[0].Status != StatusComplete || status.Stages[1].Status != StatusComplete || status.Stages[2].Status != StatusReady {
		t.Fatalf("status = %+v", status)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "flow-state.json")); err != nil {
		t.Fatalf("flow state not written: %v", err)
	}
	writeFileContent(t, sp.Path, "# Technical Handbook\n", "technical-handbook.md")
	status, err = NewService(root).Status("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if status.Stages[2].Status != StatusComplete {
		t.Fatalf("refreshed status did not observe manual artifact: %+v", status.Stages[2])
	}
	persisted, err := LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Stages[2].Status != StatusComplete {
		t.Fatalf("flow state was not synchronized after status: %+v", persisted.Stages[2])
	}
	writeFileContent(t, sp.Path, "{not json", "flow-state.json")
	if _, err := NewService(root).Status("proj", "01-alpha"); !errors.Is(err, ErrFlowStateMalformed) {
		t.Fatalf("invalid state err = %v", err)
	}
}

func workspaceFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkdirAll(t, root, "projects")
	writeFileContent(t, root, "version: 1\n", "ultraplan.yml")
	return root
}

func sprintFixture(t *testing.T, root, projectName, slug string) Sprint {
	t.Helper()
	base := filepath.Join(root, "projects", projectName)
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints", slug)
	writeFileContent(t, base, "# Roadmap\n", "roadmap.md")
	writeFileContent(t, base, "# Project Index\n", "project-index.md")
	return Sprint{Project: projectName, Slug: slug, Path: filepath.Join(base, "sprints", slug)}
}

func completeStates(sp Sprint) []StageState {
	var states []StageState
	for _, stage := range PlanningStages() {
		states = append(states, StageState{Stage: stage, Status: StatusComplete, Path: ArtifactRelPath(sp, stage)})
	}
	return states
}

func stageStrings(stages []PlanningStage) []string {
	out := make([]string, 0, len(stages))
	for _, stage := range stages {
		out = append(out, string(stage))
	}
	return out
}

func mkdirAll(t *testing.T, base string, parts ...string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(append([]string{base}, parts...)...), 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, base string, parts ...string) {
	t.Helper()
	writeFileContent(t, base, "x", parts...)
}

func writeFileContent(t *testing.T, base, content string, parts ...string) {
	t.Helper()
	path := filepath.Join(append([]string{base}, parts...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
