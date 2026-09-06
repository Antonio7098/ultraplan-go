package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func TestWebUseCasesFreshReadBoundsAndOpaqueArtifact(t *testing.T) {
	root := initializedWorkspace(t)
	projectRoot := filepath.Join(root, "projects", "alpha")
	writeFixtureFile(t, projectRoot, "docs", "PRD.md")
	writeFixtureFile(t, projectRoot, "roadmap.md")
	writeFixtureFile(t, projectRoot, "project-index.md")
	sprintRoot := filepath.Join(projectRoot, "sprints", "30-web")
	writeFixtureFile(t, sprintRoot, "requirements.md")
	writeFixtureFile(t, sprintRoot, "sprint-index.md")
	writeFixtureFile(t, sprintRoot, "technical-handbook.md")
	writeFixtureFile(t, sprintRoot, "reasoning.md")
	writeFixtureFile(t, sprintRoot, "plan.md")

	queries := NewWebUseCases(root, WebUseCaseOptions{})
	first, err := queries.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first.Workspace != filepath.Base(root) || len(first.Projects.Items) != 1 || len(first.Sprints) != 1 {
		t.Fatalf("dashboard=%+v", first)
	}
	if _, err := os.Stat(filepath.Join(sprintRoot, "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("read-only dashboard created flow state: %v", err)
	}
	project := first.Projects.Items[0]
	if project.Ref == "" || len(project.Artifacts) == 0 || strings.Contains(project.Artifacts[0].Ref, "/") {
		t.Fatalf("project/artifacts=%+v", project)
	}
	preview, err := queries.Artifact(context.Background(), project.Artifacts[0].Ref)
	if err != nil || preview.MediaType != "text/markdown" || preview.DisplayPath == "" {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}
	if _, err := queries.Artifact(context.Background(), strings.Repeat("A", 43)); !errors.Is(err, ErrWebNotFound) {
		t.Fatalf("forged ref err=%v", err)
	}

	second, err := queries.Dashboard(context.Background())
	if err != nil || second.Ref != first.Ref {
		t.Fatalf("fresh dashboard ref=%q first=%q err=%v", second.Ref, first.Ref, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := queries.Dashboard(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled dashboard err=%v", err)
	}
}

func TestWebUseCasesCodeContextPreservesArtifactAndLatestOutcome(t *testing.T) {
	root := initializedWorkspace(t)
	writeCommandSprintProject(t, root, "proj", "01-alpha")
	base := filepath.Join(root, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, "# Requirements\n\nImplement context.\n", "requirements.md")
	writeCommandCompletedCodeContext(t, root, "proj", "01-alpha")
	sp := sprint.Sprint{Project: "proj", Slug: "01-alpha", Path: base}
	state, err := sprint.LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for i := range state.Stages {
		if state.Stages[i].Stage == sprint.StageCodeContext {
			state.Stages[i].Status = sprint.StatusFailed
			state.Stages[i].LastRunAt = &now
			state.Stages[i].Error = "provider failed"
		}
	}
	if err := sprint.SaveFlowState(root, sp, state); err != nil {
		t.Fatal(err)
	}
	queries := NewWebUseCases(root, WebUseCaseOptions{})
	result, err := queries.Sprint(context.Background(), "proj", "01-alpha")
	if err != nil {
		t.Fatal(err)
	}
	foundStage := false
	for _, stage := range result.RunStages {
		if stage.Name != "code-context" {
			continue
		}
		if stage.Status != "failed" || stage.LatestOutcome != "failed" || !stage.ArtifactAvailable || !stage.ArtifactValid || !strings.Contains(stage.NextAction, "prior valid artifact") {
			t.Fatalf("code-context projection = %+v", stage)
		}
		foundStage = true
		break
	}
	if !foundStage {
		t.Fatal("code-context stage missing from web projection")
	}
	for _, artifact := range result.Artifacts {
		if artifact.Label != "code-context" {
			continue
		}
		preview, err := queries.Artifact(context.Background(), artifact.Ref)
		if err != nil || !strings.Contains(preview.Content, "# Sprint Code Context") || preview.Truncated {
			t.Fatalf("code-context preview=%+v err=%v", preview, err)
		}
		return
	}
	t.Fatal("code-context artifact missing from web projection")
}

func TestSprintRunStagesKeepsStaleQAInTheFlow(t *testing.T) {
	stages := sprintRunStages(SprintSummary{QA: QAResult{Phase: string(sprint.QAPhaseCompleted), Assessment: string(sprint.AssessmentPass)}})
	for _, stage := range stages {
		if stage.Name == "qa" {
			if stage.Status != "stale" {
				t.Fatalf("QA stage status = %q, want stale", stage.Status)
			}
			return
		}
	}
	t.Fatal("QA stage missing from sprint flow")
}

func TestWebPromptBundleIsContentFreeLazyAndReadOnly(t *testing.T) {
	root := initializedWorkspace(t)
	writeCommandSprintProject(t, root, "proj", "01-alpha")
	queries := NewWebUseCases(root, WebUseCaseOptions{})

	result, err := queries.PromptBundle(context.Background(), "proj", "01-alpha", "requirements")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available || result.Explanation == nil || result.Explanation.TotalBytes == 0 || len(result.Explanation.Blocks) == 0 {
		t.Fatalf("prompt bundle = %+v", result)
	}
	if result.InputContract.Stage != sprint.StageRequirements || strings.Join(result.InputContract.Required, ",") != "project-index,roadmap,project-docs" {
		t.Fatalf("input contract = %+v", result.InputContract)
	}
	if _, err := os.Stat(filepath.Join(root, "projects", "proj", "sprints", "01-alpha", "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("prompt observability wrote flow state: %v", err)
	}

	smoke, err := queries.PromptBundle(context.Background(), "proj", "01-alpha", "smoke")
	if err != nil || smoke.Available || smoke.UnavailableReason == "" || smoke.InputContract.Stage != sprint.StageSmoke {
		t.Fatalf("smoke bundle = %+v err=%v", smoke, err)
	}
	if _, err := queries.PromptBundle(context.Background(), "proj", "01-alpha", "unknown"); !errors.Is(err, ErrWebNotFound) {
		t.Fatalf("unknown stage err=%v", err)
	}
}

func TestWebArtifactTruncationJSONAndSymlinkEscape(t *testing.T) {
	root := initializedWorkspace(t)
	sprintRoot := filepath.Join(root, "projects", "alpha", "sprints", "30-web")
	writeFixtureFile(t, filepath.Join(root, "projects", "alpha"), "docs", "PRD.md")
	writeFixtureFile(t, filepath.Join(root, "projects", "alpha"), "roadmap.md")
	writeFixtureFile(t, filepath.Join(root, "projects", "alpha"), "project-index.md")
	content := strings.Repeat("x", WebPreviewByteLimit+7)
	writeFixtureFileContent(t, sprintRoot, content, "flow-state.json")

	queries := NewWebUseCases(root, WebUseCaseOptions{})
	dashboard, err := queries.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ref string
	for _, artifact := range dashboard.Sprints[0].Artifacts {
		if strings.HasSuffix(artifact.DisplayPath, "flow-state.json") {
			ref = artifact.Ref
		}
	}
	if ref == "" {
		t.Fatal("flow-state artifact ref missing")
	}
	preview, err := queries.Artifact(context.Background(), ref)
	if err != nil || !preview.Truncated || preview.ReturnedBytes != WebPreviewByteLimit || preview.JSONValid {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}

	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(sprintRoot, "review.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	dashboard, err = queries.Dashboard(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range dashboard.Sprints[0].Artifacts {
		if strings.HasSuffix(artifact.DisplayPath, "review.md") {
			if _, err := queries.Artifact(context.Background(), artifact.Ref); !errors.Is(err, ErrWebNotFound) {
				t.Fatalf("symlink escape err=%v", err)
			}
			return
		}
	}
	t.Fatal("review artifact ref missing")
}

func TestWebArtifactExactLimitAndRejectedClasses(t *testing.T) {
	root := initializedWorkspace(t)
	sprintRoot := filepath.Join(root, "projects", "alpha", "sprints", "30-web")
	writeFixtureFile(t, filepath.Join(root, "projects", "alpha"), "docs", "PRD.md")
	writeFixtureFile(t, filepath.Join(root, "projects", "alpha"), "roadmap.md")
	writeFixtureFile(t, filepath.Join(root, "projects", "alpha"), "project-index.md")
	exact := strings.Repeat("m", WebPreviewByteLimit)
	writeFixtureFileContent(t, sprintRoot, exact, "plan.md")

	concrete := NewWebUseCases(root, WebUseCaseOptions{}).(*webUseCases)
	validRef := concrete.issue("artifact", "projects/alpha/sprints/30-web/plan.md")
	preview, err := concrete.Artifact(context.Background(), validRef)
	if err != nil || preview.Truncated || preview.ReturnedBytes != WebPreviewByteLimit {
		t.Fatalf("exact preview=%+v err=%v", preview, err)
	}
	for _, rel := range []string{
		"../outside.md",
		"/etc/passwd",
		"projects/alpha/secret.txt",
		"projects/alpha/sprints/30-web/../../outside.md",
	} {
		ref := concrete.issue("artifact", rel)
		if _, err := concrete.Artifact(context.Background(), ref); !errors.Is(err, ErrWebNotFound) {
			t.Errorf("%q err=%v", rel, err)
		}
	}
}

func TestWebProjectsCollectionLimitAndHealth(t *testing.T) {
	root := initializedWorkspace(t)
	for i := 0; i < WebCollectionLimit+1; i++ {
		name := filepath.Join(root, "projects", fmt.Sprintf("project-%03d", i))
		writeFixtureFile(t, name, "docs", "PRD.md")
		writeFixtureFile(t, name, "roadmap.md")
		writeFixtureFile(t, name, "project-index.md")
		if err := os.MkdirAll(filepath.Join(name, "sprints"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	queries := NewWebUseCases(root, WebUseCaseOptions{})
	projects, err := queries.Projects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(projects.Items) != WebCollectionLimit || projects.TotalCount != WebCollectionLimit+1 || !projects.Truncated {
		t.Fatalf("projects counts=%+v len=%d", projects.CollectionInfo, len(projects.Items))
	}
	health, err := queries.Health(context.Background())
	if err != nil || health.Status != "ok" || !health.Server || !health.Workspace {
		t.Fatalf("health=%+v err=%v", health, err)
	}
}

func TestWebEmptyCollectionsAreKnownAndNonNil(t *testing.T) {
	root := initializedWorkspace(t)
	queries := NewWebUseCases(root, WebUseCaseOptions{})
	projects, err := queries.Projects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	studies, err := queries.Studies(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if projects.Items == nil || studies.Items == nil || projects.TotalCount != 0 || studies.TotalCount != 0 {
		t.Fatalf("projects=%+v studies=%+v", projects, studies)
	}
}

func TestWebSprintSurfacesStageModels(t *testing.T) {
	root := initializedWorkspace(t)
	writeCommandSprintProject(t, root, "proj", "01-alpha")
	base := filepath.Join(root, "projects", "proj", "sprints", "01-alpha")
	metrics := sprint.SprintRuntimeMetrics{
		SchemaVersion: 1, Project: "proj", Sprint: "01-alpha",
		Runs: []sprint.SprintRuntimeMetric{
			{Stage: sprint.StageReasoning, Status: "failed", Model: "provider/old"},
			{Stage: sprint.StageReasoning, Status: "completed", Model: "openai/gpt-5.6-sol"},
			{Stage: sprint.StageRequirements, Status: "completed", Model: "workspace/primary"},
			{Stage: sprint.StagePlan, Status: "completed", Model: ""},
		},
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFileContent(t, base, string(data), ".runtime-metrics.json")
	for _, artifact := range []string{"requirements.md", "sprint-index.md", "technical-handbook.md", "reasoning.md", "plan.md"} {
		writeFixtureFileContent(t, base, "# "+artifact+"\n", artifact)
	}

	queries := NewWebUseCases(root, WebUseCaseOptions{StageRuntime: map[sprint.PlanningStage]sprint.StageRuntime{
		sprint.StageReasoning: {Model: "openai/gpt-5.6-sol", Variant: "high"},
	}, QASettings: sprint.QASettings{Runtime: sprint.StageRuntime{Model: "openai/qa"}}})
	result, err := queries.Sprint(context.Background(), "proj", "01-alpha")
	if err != nil {
		t.Fatal(err)
	}
	models := map[string]StageSummary{}
	for _, stage := range result.RunStages {
		models[stage.Name] = stage
	}
	reasoning := models["reasoning"]
	if reasoning.ConfiguredModel != "openai/gpt-5.6-sol" || reasoning.RunModel != "openai/gpt-5.6-sol" {
		t.Fatalf("reasoning stage = %+v", reasoning)
	}
	requirements := models["requirements"]
	if requirements.ConfiguredModel != "" || requirements.RunModel != "workspace/primary" {
		t.Fatalf("requirements stage = %+v", requirements)
	}
	if plan := models["plan"]; plan.RunModel != "" {
		t.Fatalf("plan stage should have no recorded run model = %+v", plan)
	}
	if qa := models["qa"]; qa.ConfiguredModel != "openai/qa" {
		t.Fatalf("QA configured model = %+v", qa)
	}
}

func TestWebSprintRuntimeUsageReturnsCompleteLedgerAndRecentWindow(t *testing.T) {
	root := initializedWorkspace(t)
	writeCommandSprintProject(t, root, "proj", "01-alpha")
	base := filepath.Join(root, "projects", "proj", "sprints", "01-alpha")
	metrics := sprint.SprintRuntimeMetrics{SchemaVersion: 2, Project: "proj", Sprint: "01-alpha"}
	for index := 1; index <= 14; index++ {
		metrics.Runs = append(metrics.Runs, sprint.SprintRuntimeMetric{
			Sequence: index, StageSequence: index, Stage: sprint.PlanningStage(sprint.VerificationPhaseQA), Role: "arbiter", CallID: fmt.Sprintf("call-%d", index),
			Provider: "openrouter", Model: "minimax/minimax-m3:free", Variant: "high", ToolCalls: index, ToolCallCountExact: true,
			InputTokens: sprint.RuntimeTokenMetric{Known: true, Value: int64(index)}, CacheReadTokens: sprint.RuntimeTokenMetric{Known: true, Value: int64(index * 10)},
		})
	}
	data, err := json.Marshal(metrics)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFileContent(t, base, string(data), ".runtime-metrics.json")

	summary, err := NewWebUseCases(root, WebUseCaseOptions{}).SprintRuntimeUsage(context.Background(), "proj", "01-alpha")
	if err != nil {
		t.Fatal(err)
	}
	if summary.SchemaVersion != 2 || len(summary.AllRuns) != 14 || len(summary.RecentRuns) != 12 {
		t.Fatalf("summary = %+v", summary)
	}
	last := summary.AllRuns[len(summary.AllRuns)-1]
	if last.CallID != "call-14" || last.Role != "arbiter" || last.Provider != "openrouter" || last.Variant != "high" || last.ToolCalls != 14 || !last.ToolCallCountExact || last.CacheRead != 140 {
		t.Fatalf("last call = %+v", last)
	}
}

func TestWebProjectRoadmapJoinsLiveSprintState(t *testing.T) {
	root := initializedWorkspace(t)
	base := filepath.Join(root, "projects", "alpha")
	writeFixtureFile(t, base, "docs", "PRD.md")
	writeFixtureFileContent(t, base, "# Project Index\n", "project-index.md")
	roadmap := structuredRoadmapFixture("alpha") + `
### Sprint 2: Planned Sprint

> Slug: 02-planned
> Status: planned
> Depends On: 1

#### Goal

Not yet materialized.

#### Build

- future work

#### Acceptance

- [ ] later
`
	writeFixtureFileContent(t, base, roadmap, "roadmap.md")
	sprintRoot := filepath.Join(base, "sprints", "01-test")
	writeFixtureFileContent(t, sprintRoot, `{"schemaVersion":1}`, "flow-state.json")

	queries := NewWebUseCases(root, WebUseCaseOptions{})
	result, err := queries.Project(context.Background(), "alpha")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Roadmap) != 1 || len(result.Roadmap[0].Sprints) != 2 {
		t.Fatalf("roadmap = %+v", result.Roadmap)
	}
	first := result.Roadmap[0].Sprints[0]
	if first.Number != 2 || first.Exists || first.Status != "planned" || first.DependsOn[0] != 1 {
		t.Fatalf("newest entry = %+v", first)
	}
	second := result.Roadmap[0].Sprints[1]
	if second.Number != 1 || second.Slug != "01-test" || !second.Exists {
		t.Fatalf("materialized entry = %+v", second)
	}
	if second.Goal != "Prove the fixture project." || len(second.GateItems) != 1 || second.GateItems[0] != "fixture validates" {
		t.Fatalf("entry content = %+v", second)
	}
}
