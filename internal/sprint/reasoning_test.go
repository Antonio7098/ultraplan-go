package sprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
)

func TestReasoningManifestPromptsAndValidation(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeFileContent(t, root, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo reasoning stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	writeFileContent(t, sp.Path, validFinalReasoning(), "reasoning.md")

	service := NewService(root).WithRuntime(panicRuntime{})
	areaPreview, err := service.PromptAreaReasoning("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Create Area Reasoning", "Prompt source: `builtin:prompts/create-area-reasoning.md`", "Injected Selected Reasoning Template:", "# Architecture Template", "Architecture", "projects/proj/sprints/01-alpha/reasoning/architecture.md", "Use the injected selected reasoning template section as source material", "## Area Decisions", "## Trade-Offs", "## Evidence", "## Risks", "Do not write final reasoning.md"} {
		if !strings.Contains(areaPreview.Prompt, want) {
			t.Fatalf("area prompt missing %q:\n%s", want, areaPreview.Prompt)
		}
	}
	finalPreview, err := service.PromptReasoning("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Create Sprint Reasoning", "Prompt source: `builtin:prompts/create-sprint-reasoning.md`", "Injected Sprint Reasoning Template:", "Source: builtin:templates/sprint-reasoning.md", "Required selected area reasoning", "Use only selected context", "Do not generate or validate plan.md"} {
		if !strings.Contains(finalPreview.Prompt, want) {
			t.Fatalf("final prompt missing %q:\n%s", want, finalPreview.Prompt)
		}
	}
	if strings.Contains(areaPreview.Prompt+finalPreview.Prompt, root) || strings.Contains(areaPreview.Prompt+finalPreview.Prompt, "\x1b[") {
		t.Fatalf("prompt leaked unsafe output")
	}
	area, err := service.ValidateAreaReasoning("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if !area.Valid() {
		t.Fatalf("area findings = %+v", area.Findings)
	}
	final, err := service.ValidateReasoning("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if !final.Valid() {
		t.Fatalf("final findings = %+v", final.Findings)
	}

	writeFileContent(t, sp.Path, strings.Replace(strings.Replace(validFinalReasoning(), "projects/proj/sprints/01-alpha/reasoning/architecture.md", "08-concurrency", 1), "Architecture", "Unselected", 1), "reasoning.md")
	final, err = service.ValidateReasoning("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if final.Valid() {
		t.Fatalf("expected missing selected area reference")
	}
}

func TestReasoningPromptsUseProjectThenWorkspaceThenBuiltinDefaults(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo reasoning stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, root, "# Workspace Area Prompt\n", "prompts", "create-area-reasoning.md")
	writeFileContent(t, root, "# Project Area Prompt\n", "projects", "proj", "prompts", "create-area-reasoning.md")
	writeFileContent(t, root, "# Project Final Prompt\n", "projects", "proj", "prompts", "create-sprint-reasoning.md")
	writeFileContent(t, root, "# Project Final Template\n", "projects", "proj", "templates", "sprint-reasoning.md")

	service := NewService(root).WithRuntime(panicRuntime{})
	area, err := service.PromptAreaReasoning("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Project Area Prompt",
		"Prompt source: `project:projects/proj/prompts/create-area-reasoning.md`",
	} {
		if !strings.Contains(area.Prompt, want) {
			t.Fatalf("area prompt missing %q:\n%s", want, area.Prompt)
		}
	}
	if strings.Contains(area.Prompt, "# Workspace Area Prompt") {
		t.Fatalf("project prompt did not shadow workspace prompt:\n%s", area.Prompt)
	}

	final, err := service.PromptReasoning("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# Project Final Prompt",
		"Prompt source: `project:projects/proj/prompts/create-sprint-reasoning.md`",
		"Source: project:projects/proj/templates/sprint-reasoning.md",
		"# Project Final Template",
	} {
		if !strings.Contains(final.Prompt, want) {
			t.Fatalf("final prompt missing %q:\n%s", want, final.Prompt)
		}
	}
}

func TestReasoningManifestRejectsUnsafeOrUnreadableTemplates(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo reasoning stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	catalog, _ := project.ParseProjectIndex(testProjectIndex())
	inputs, err := NewFSStore(root).ReadPlanningInputs(sp)
	if err != nil {
		t.Fatal(err)
	}
	_, findings := BuildReasoningManifest(root, sp, inputs, catalog)
	if len(findings) == 0 {
		t.Fatalf("expected unreadable template finding")
	}
}

func TestFlowReasoningDryRunSuccessAndFailure(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeFileContent(t, root, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo reasoning stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")

	service := NewService(root).WithRuntime(panicRuntime{})
	result, err := service.FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageReasoning, DryRun: true})
	if err == nil || len(result.Findings) == 0 {
		t.Fatalf("expected missing area prerequisite, result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run prerequisite failure should not require runtime, state err=%v", err)
	}

	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	result, err = service.FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageReasoning, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || !strings.Contains(result.Message, "# Create Sprint Reasoning") {
		t.Fatalf("dry-run result = %+v", result)
	}

	writer := writeReasoningRuntime{root: root, sp: sp}
	service = NewService(root).WithRuntime(writer)
	result, err = service.FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageReasoning})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stages[4].Status != StatusComplete || result.Stages[5].Status == StatusComplete {
		t.Fatalf("stages = %+v", result.Stages)
	}
}

func TestFlowReasoningRepairsInvalidGeneratedArtifactInSameSession(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeFileContent(t, root, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFileContent(t, sp.Path, "# Requirements\n\nDo reasoning stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")

	rt := &repairReasoningRuntime{sp: sp}
	service := NewService(root).WithRuntime(rt)
	result, err := service.FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageReasoning})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stages[4].Status != StatusComplete {
		t.Fatalf("stages = %+v", result.Stages)
	}
	if len(rt.requests) != 2 {
		t.Fatalf("runtime starts = %d, want initial plus repair", len(rt.requests))
	}
	if rt.requests[1].SessionID != "session-1" || rt.requests[1].SessionAction != "continue" {
		t.Fatalf("repair request session id=%q action=%q", rt.requests[1].SessionID, rt.requests[1].SessionAction)
	}
	data, err := os.ReadFile(filepath.Join(sp.Path, "reasoning.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "## Decisions") {
		t.Fatalf("repair did not write valid decisions section:\n%s", data)
	}
}

type writeReasoningRuntime struct {
	root string
	sp   Sprint
}

func (r writeReasoningRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	if req.Metadata["stage"] == string(StageReasoning) {
		if err := os.WriteFile(filepath.Join(r.sp.Path, "reasoning.md"), []byte(validFinalReasoning()), 0o644); err != nil {
			return pruntime.Result{}, err
		}
	}
	return pruntime.Result{Status: "success", RunID: "reason-run"}, nil
}

type repairReasoningRuntime struct {
	sp       Sprint
	requests []pruntime.Request
}

func (r *repairReasoningRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.requests = append(r.requests, req)
	content := strings.Replace(validFinalReasoning(), "## Decisions", "## Final Decisions", 1)
	if req.SessionAction == "continue" {
		content = validFinalReasoning()
	}
	if err := os.WriteFile(filepath.Join(r.sp.Path, "reasoning.md"), []byte(content), 0o644); err != nil {
		return pruntime.Result{}, err
	}
	return pruntime.Result{Status: "success", RunID: "reason-run", SessionID: "session-1"}, nil
}

func validReasoningTechnicalHandbook() string {
	return `# Sprint Technical Handbook

## Selected Studies And Reports

| Study / Report | Path | Relevant Finding |
| --- | --- | --- |
| 01-project-structure | .ultra/studies/go-cli-study/reports/final/01-project-structure.md | Thin entrypoints. |

## Relevant Patterns

- Module-owned behavior.

## Trade-Offs

| Trade-Off | Benefit | Cost |
| --- | --- | --- |
| Local validation | Clear ownership | Focused parser |

## Anti-Patterns And Warnings

- Do not read unselected evidence.

## Open Questions For Reasoning

- How strict should validation be?

## Evidence Pointers

- .ultra/studies/go-cli-study/reports/final/01-project-structure.md
`
}

func validAreaReasoning() string {
	return `# Architecture Reasoning

## Area Decisions

- Architecture stays in internal/sprint and uses .ultra/system/reasoning/architecture_reasoning_template.md.

## Trade-Offs

- Local validation keeps boundaries clear.

## Evidence

- Architecture uses selected report .ultra/studies/go-cli-study/reports/final/01-project-structure.md.

## Risks

- Validation remains structural.
`
}

func validFinalReasoning() string {
	return `# Sprint Reasoning

## Sprint Purpose

Implement reasoning stage.

## Selected Context And Pre-Reasoning Artifacts

- requirements.md
- sprint-index.md
- technical-handbook.md

## Area-Specific Reasoning Inputs

- Architecture: projects/proj/sprints/01-alpha/reasoning/architecture.md

## Decisions

- Keep reason-stage behavior in internal/sprint.

## Expected Evidence

- go test ./internal/sprint

## Assumptions And Risks

- Structural validators cannot prove all prose semantics.

## Implementation Constraints

- Do not generate or validate plan.md.
`
}
