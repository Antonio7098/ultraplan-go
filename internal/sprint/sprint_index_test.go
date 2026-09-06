package sprint

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
)

func TestSprintIndexParseAndValidateAgainstCatalog(t *testing.T) {
	catalog, _ := project.ParseProjectIndex(testProjectIndex())
	_, findings := ValidateSprintIndexContent(validSprintIndex(), catalog)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}

	_, findings = ValidateSprintIndexContent(strings.Replace(validSprintIndex(), "Architecture", "Unknown", 1), catalog)
	if len(findings) == 0 {
		t.Fatalf("expected invalid selected contract")
	}
	if findings[0].Section == "" || findings[0].Problem == "" || findings[0].Suggestion == "" {
		t.Fatalf("finding is not actionable: %+v", findings[0])
	}

	_, findings = ParseSprintIndex(strings.Replace(validSprintIndex(), "| Architecture |", "| Architecture |\n| Architecture |", 1))
	if len(findings) == 0 || !strings.Contains(findings[0].Problem, "duplicate") {
		t.Fatalf("duplicate findings = %+v", findings)
	}
}

func TestPromptPreviewAndFlowDryRunAreRuntimeFree(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo select stage.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")

	service := NewService(root).WithRuntime(panicRuntime{})
	statePath := filepath.Join(sp.Path, "flow-state.json")
	stateBefore, _ := os.ReadFile(statePath)
	preview, err := service.PromptSprintIndex("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"projects/proj/sprints/01-alpha/requirements.md", "projects/proj/sprints/01-alpha/sprint-index.md", "Do not mutate", "Architecture", "Sprint Review"} {
		if !strings.Contains(preview.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, preview.Prompt)
		}
	}
	if strings.Contains(preview.Prompt, root) || strings.Contains(preview.Prompt, "\x1b[") {
		t.Fatalf("prompt leaked absolute path or ANSI: %q", preview.Prompt)
	}

	result, err := service.FlowSprintIndex(context.Background(), "proj", "01", FlowRequest{To: StageSprintIndex, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Message == "" {
		t.Fatalf("dry run result = %+v", result)
	}
	stateAfter, stateErr := os.ReadFile(statePath)
	if stateErr != nil || string(stateAfter) != string(stateBefore) {
		t.Fatalf("dry-run changed flow state: %v", stateErr)
	}
}

func TestFlowSuccessAndValidationFailureUpdateState(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo select stage.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")

	service := NewService(root).WithRuntime(fakeRuntime{})
	result, err := service.FlowSprintIndex(context.Background(), "proj", "01", FlowRequest{To: StageSprintIndex})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stages[2].Status != StatusComplete || result.Stages[3].Status != StatusReady {
		t.Fatalf("stages = %+v", result.Stages)
	}

	writeFileContent(t, sp.Path, "# Sprint Index\n\nTODO\n", "sprint-index.md")
	result, err = service.FlowSprintIndex(context.Background(), "proj", "01", FlowRequest{To: StageSprintIndex})
	if err == nil || len(result.Findings) == 0 || result.Stages[2].Status != StatusFailed {
		t.Fatalf("expected validation failure, result=%+v err=%v", result, err)
	}
}

func TestFlowConfiguresRuntimeValidationAndKeepsProductValidation(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo select stage.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, "# Sprint Index\n\nTODO\n", "sprint-index.md")

	rt := &validationInspectRuntime{}
	service := NewService(root).WithRuntime(rt)
	_, _ = service.FlowSprintIndex(context.Background(), "proj", "01", FlowRequest{To: StageSprintIndex})
	if rt.request.Validation == nil || len(rt.request.Validation.Validators) == 0 || rt.request.Validation.Repair.MaxAttempts != 2 {
		t.Fatalf("runtime request validation = %#v, want semantic validation with bounded repair", rt.request.Validation)
	}
}

func TestFlowCreatesMissingSprintSkeletonOnlyWhenNotDryRun(t *testing.T) {
	root := workspaceFixture(t)
	writeFixtureProjectIndex(t, root, "proj")
	service := NewService(root)

	_, err := service.FlowRequirements(context.Background(), "proj", "23-execute-stage", FlowRequest{To: StageRequirements, DryRun: true})
	if err == nil {
		t.Fatal("expected dry-run missing sprint error")
	}
	if _, statErr := os.Stat(filepath.Join(root, "projects", "proj", "sprints", "23-execute-stage")); !os.IsNotExist(statErr) {
		t.Fatalf("dry-run created sprint: %v", statErr)
	}

	result, err := service.FlowRequirements(context.Background(), "proj", "23-execute-stage", FlowRequest{To: StageRequirements})
	if err == nil || !strings.Contains(err.Error(), "runtime is required") {
		t.Fatalf("expected runtime error after skeleton creation, result=%+v err=%v", result, err)
	}
	if _, statErr := os.Stat(filepath.Join(root, "projects", "proj", "sprints", "23-execute-stage")); statErr != nil {
		t.Fatalf("sprint directory not created: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(root, "projects", "proj", "sprints", "23-execute-stage", "requirements.md")); !os.IsNotExist(statErr) {
		t.Fatalf("requirements should be runtime-generated, stat=%v", statErr)
	}
}

func TestFlowRequirementsGeneratesMissingSprintRequirements(t *testing.T) {
	root := workspaceFixture(t)
	writeFixtureProjectIndex(t, root, "proj")
	reqPath := filepath.Join(root, "projects", "proj", "sprints", "23-execute-stage", "requirements.md")
	service := NewService(root).WithRuntime(writeFileRuntime{Path: reqPath, Content: validRequirements("proj", "23-execute-stage")})

	result, err := service.FlowRequirements(context.Background(), "proj", "23-execute-stage", FlowRequest{To: StageRequirements})
	if err != nil {
		t.Fatalf("flow requirements failed: result=%+v err=%v", result, err)
	}
	data, readErr := os.ReadFile(reqPath)
	if readErr != nil {
		t.Fatalf("requirements not created: %v", readErr)
	}
	content := string(data)
	if !strings.Contains(content, "Execute validated plan tasks") || containsPlaceholder(content) {
		t.Fatalf("unexpected requirements:\n%s", content)
	}
	if result.Stages[0].Status != StatusComplete || result.Stages[1].Status != StatusReady {
		t.Fatalf("unexpected stages: %+v", result.Stages)
	}
}

func TestCumulativeFlowMaterializesMissingSprintBeforeMutationLock(t *testing.T) {
	root := workspaceFixture(t)
	writeFixtureProjectIndex(t, root, "proj")
	service := NewService(root)
	_, err := service.Flow(context.Background(), "proj", "33-code-context-stage", FlowRequest{To: StagePlan})
	if err == nil || !strings.Contains(err.Error(), "runtime is required") {
		t.Fatalf("flow error=%v, want requirements runtime failure after materialization", err)
	}
	path := filepath.Join(root, "projects", "proj", "sprints", "33-code-context-stage")
	if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
		t.Fatalf("missing sprint skeleton %s: info=%v err=%v", path, info, statErr)
	}
}

func TestFlowToPlanSchedulesCodeContextExactlyOnceInCanonicalOrder(t *testing.T) {
	stages, err := flowStages(StagePlan)
	if err != nil {
		t.Fatal(err)
	}
	want := []PlanningStage{StageRequirements, StageCodeContext, StageSprintIndex, StageTechnicalHandbook, StageAreaReasoning, StageReasoning, StagePlan}
	if len(stages) != len(want) {
		t.Fatalf("flow stages = %v, want %v", stages, want)
	}
	count := 0
	for i := range want {
		if stages[i] != want[i] {
			t.Fatalf("flow stages = %v, want %v", stages, want)
		}
		if stages[i] == StageCodeContext {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("code-context scheduled %d times", count)
	}
}

func TestFlowTargetsQAInsteadOfSmoke(t *testing.T) {
	stages, err := flowStages(StageQA)
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) != len(PlanningStages())+1 || stages[len(stages)-1] != StageExecute {
		t.Fatalf("QA flow prerequisites = %v", stages)
	}
	if err := validateFlowTarget(StageQA); err != nil {
		t.Fatalf("QA flow target rejected: %v", err)
	}
	if err := validateFlowTarget(StageSmoke); err == nil {
		t.Fatal("smoke remains accepted as a flow target")
	}
}

type fakeRuntime struct{}

func (fakeRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	return pruntime.Result{Status: "success", RunID: "run-1"}, nil
}

type validationInspectRuntime struct {
	request pruntime.Request
}

func (r *validationInspectRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.request = req
	return pruntime.Result{Status: "success", RunID: "run-1"}, nil
}

type writeFileRuntime struct {
	Path    string
	Content string
}

func (r writeFileRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o755); err != nil {
		return pruntime.Result{}, err
	}
	if err := os.WriteFile(r.Path, []byte(r.Content), 0o644); err != nil {
		return pruntime.Result{}, err
	}
	return pruntime.Result{Status: "success", RunID: "run-1"}, nil
}

func validRequirements(projectName, sprintSlug string) string {
	return fmt.Sprintf(`# Sprint Requirements: %s

> Project: %s
> Sprint: %s

## Sprint Goal

Execute validated plan tasks through the runtime boundary.

## Required Outputs

| Output | Path | Description |
| --- | --- | --- |
| Sprint index | projects/%s/sprints/%s/sprint-index.md | Selected context. |

## Acceptance Criteria

- [ ] Requirements are sprint-specific.

## Non-Goals

- Smoke investigation.

## Constraints

- Use workspace-relative paths.

## Dependencies

| Prior Sprint / Output | Required For | Notes |
| --- | --- | --- |
| Project index | Planning | Must validate. |

## Review Expectations

| What | How Verified |
| --- | --- |
| Requirements | Validate the requirements stage. |
`, sprintSlug, projectName, sprintSlug, projectName, sprintSlug)
}

type panicRuntime struct{}

func (panicRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	panic("runtime should not be called")
}

func writeFixtureProjectIndex(t *testing.T, root, projectName string) {
	t.Helper()
	base := filepath.Join(root, "projects", projectName)
	writeFileContent(t, base, testProjectIndex(), "project-index.md")
	writeFileContent(t, base, "# PRD\n", "docs", "PRD.md")
}

func testProjectIndex() string {
	return `# Project Index

## Project Scope

- **Target Implementation Directory:** /home/antonioborgerees/coding/ultraplan/ultraplan-go

## Active Contract Pool

| Contract | Path | Applies To |
|---|---|---|
| Architecture | .ultra/system/contracts/core/architecture.md | All sprints |
| Errors | .ultra/system/contracts/core/errors.md | All sprints |

## Available Evidence Reports

| Report | Path | Covers |
|---|---|---|
| 01-project-structure | .ultra/studies/go-cli-study/reports/final/01-project-structure.md | Project layout |

## Available Reasoning Templates

| Template | Path | Useful For |
|---|---|---|
| Architecture | .ultra/system/reasoning/architecture_reasoning_template.md | Boundaries |

## Review Protocols

| Protocol | Path | Required When |
|---|---|---|
| Sprint Review | .ultra/system/protocols/sprint-review-protocol.md | Every sprint |
`
}

func validSprintIndex() string {
	return `# Sprint Index

## Selected Contracts

| Contract | Why Selected |
|---|---|
| Architecture | Boundaries |

## Selected Evidence Reports

| Report | Path | Covers |
|---|---|---|
| 01-project-structure | .ultra/studies/go-cli-study/reports/final/01-project-structure.md | Project layout |

## Selected Reasoning Templates

| Template | Output Path | Why Selected |
|---|---|---|
| Architecture | projects/proj/sprints/01-alpha/reasoning/architecture.md | Boundaries |

## Required Review Protocols

| Protocol | Path | Required Evidence |
|---|---|---|
| Sprint Review | .ultra/system/protocols/sprint-review-protocol.md | Evidence |

## Excluded Context

| Context | Reason Excluded | Revisit If |
|---|---|---|
| Sprint implementation execution | deferred | future |
| Smoke investigation execution | deferred | future |
| Automated review | deferred | future |
| Issue tracking | deferred | future |
| Git mutation | deferred | future |
`
}
