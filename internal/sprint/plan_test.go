package sprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestPlanValidationPromptAndFlow(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeFileContent(t, root, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo plan stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	writeFileContent(t, sp.Path, validPlanFinalReasoning(), "reasoning.md")
	writeFileContent(t, sp.Path, validPlan(), "plan.md")

	service := NewService(root).WithRuntime(panicRuntime{})
	result, err := service.ValidatePlan("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid() {
		t.Fatalf("plan findings = %+v", result.Findings)
	}
	preview, err := service.PromptPlan("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Generate plan.md", "projects/proj/sprints/01-alpha/plan.md", "Selected area reasoning", "Trace every executable task", "Do not execute implementation tasks"} {
		if !strings.Contains(preview.Prompt, want) {
			t.Fatalf("plan prompt missing %q:\n%s", want, preview.Prompt)
		}
	}
	if strings.Contains(preview.Prompt, root) || strings.Contains(preview.Prompt, "\x1b[") {
		t.Fatalf("prompt leaked unsafe output")
	}
	_ = os.Remove(filepath.Join(sp.Path, "plan.md"))
	dry, err := service.FlowPlan(context.Background(), "proj", "01", FlowRequest{To: StagePlan, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun || !strings.Contains(dry.Message, "Generate plan.md") {
		t.Fatalf("dry run = %+v", dry)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote state: %v", err)
	}
	writer := writePlanRuntime{sp: sp}
	service = NewService(root).WithRuntime(writer)
	flow, err := service.FlowPlan(context.Background(), "proj", "01", FlowRequest{To: StagePlan})
	if err != nil {
		t.Fatal(err)
	}
	if flow.Stages[5].Status != StatusComplete {
		t.Fatalf("stages = %+v", flow.Stages)
	}
}

func TestPlanValidationFailures(t *testing.T) {
	manifest := PlanManifest{
		OutputPath:      "projects/proj/sprints/01-alpha/plan.md",
		ReasoningPath:   "projects/proj/sprints/01-alpha/reasoning.md",
		DecisionNames:   []string{"Keep Plan Behavior In Sprint"},
		EvidencePhrases: []string{"go test ./..."},
	}
	cases := map[string]string{
		"empty":             "",
		"placeholder":       strings.Replace(validPlan(), "Task 1", "TODO", 1),
		"missing reasoning": strings.ReplaceAll(validPlan(), "reasoning.md", "source.md"),
		"missing decisions": strings.Replace(validPlan(), "## Decisions To Execute", "## Other", 1),
		"missing tasks":     strings.Replace(validPlan(), "## Tasks", "## Work", 1),
		"missing evidence":  strings.Replace(validPlan(), "## Evidence Checklist", "## Evidence", 1),
		"missing risks":     strings.Replace(validPlan(), "## Risks And Blockers", "## Risks", 1),
		"missing criteria":  strings.Replace(validPlan(), "## Completion Criteria", "## Done", 1),
		"untraced task":     strings.Replace(validPlan(), " for Decision 1 / AC-01", "", 1),
		"forbidden content": validPlan() + "\n- Run flow --to implementation.\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if len(ValidatePlanContent(content, manifest)) == 0 {
				t.Fatalf("expected validation findings")
			}
		})
	}
}

type writePlanRuntime struct {
	sp Sprint
}

func (r writePlanRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	if req.Metadata["stage"] == string(StagePlan) {
		if err := os.WriteFile(filepath.Join(r.sp.Path, "plan.md"), []byte(validPlan()), 0o644); err != nil {
			return pruntime.Result{}, err
		}
	}
	return pruntime.Result{Status: "success", RunID: "plan-run"}, nil
}

func validPlanFinalReasoning() string {
	return `# Sprint Reasoning

## Sprint Purpose

Implement plan stage.

## Selected Context And Pre-Reasoning Artifacts

- requirements.md

## Area-Specific Reasoning Inputs

- Architecture: projects/proj/sprints/01-alpha/reasoning/architecture.md

## Decisions

### Decision 1: Keep Plan Behavior In Sprint

- Decision: Plan behavior lives in internal/sprint.

## Expected Evidence

- Plan tests run with go test ./...

## Assumptions And Risks

- Structural validation has limits.

## Implementation Constraints

- Do not execute implementation tasks from plan.md.
`
}

func validPlan() string {
	return `# Sprint Plan

## Reasoning Source

- Source: projects/proj/sprints/01-alpha/reasoning.md

## Sprint Status

- Status: not started

## Decisions To Execute

| Decision | Source |
| --- | --- |
| Keep Plan Behavior In Sprint | reasoning.md |

## Requirements / Contracts To Satisfy

| Contract | Evidence |
| --- | --- |
| AC-01 | Plan tests run with go test ./... |

## Tasks

- [ ] Task 1: Add plan behavior for Decision 1 / AC-01
  > Executes: Decision 1, AC-01
  - [ ] Verification expectation: go test ./...

## Evidence Checklist

- [ ] Plan validation tests pass.

## Risks And Blockers

| Risk | Mitigation |
| --- | --- |
| Structural validation | Keep checks focused. |

## Execution Log

| Step | Evidence |
| --- | --- |
| pending | pending |

## Completion Criteria

- [ ] Tests pass.
`
}
