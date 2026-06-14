package sprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "ultraplan-go/internal/platform/runtime"
	"ultraplan-go/internal/project"
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
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")

	service := NewService(root).WithRuntime(panicRuntime{})
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
	if _, err := os.Stat(filepath.Join(sp.Path, "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote flow state: %v", err)
	}
}

func TestFlowSuccessAndValidationFailureUpdateState(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo select stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")

	service := NewService(root).WithRuntime(fakeRuntime{})
	result, err := service.FlowSprintIndex(context.Background(), "proj", "01", FlowRequest{To: StageSprintIndex})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stages[1].Status != StatusComplete || result.Stages[2].Status != StatusReady {
		t.Fatalf("stages = %+v", result.Stages)
	}

	writeFileContent(t, sp.Path, "# Sprint Index\n\nTODO\n", "sprint-index.md")
	result, err = service.FlowSprintIndex(context.Background(), "proj", "01", FlowRequest{To: StageSprintIndex})
	if err == nil || len(result.Findings) == 0 || result.Stages[1].Status != StatusFailed {
		t.Fatalf("expected validation failure, result=%+v err=%v", result, err)
	}
}

type fakeRuntime struct{}

func (fakeRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	return pruntime.Result{Status: "success", RunID: "run-1"}, nil
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
| Architecture | .ultra/system/reasoning/architecture_reasoning_template.md | Boundaries |

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
