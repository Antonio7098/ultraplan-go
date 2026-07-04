package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func TestSprintHelpIsRegistered(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"--help"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "sprint")
	for _, args := range [][]string{
		{"sprint", "--help"},
		{"sprint", "proj", "01", "status", "--help"},
	} {
		stdout, stderr, status = runForTest(args)
		if status != ExitOK || stderr != "" {
			t.Fatalf("%v status = %d stderr = %q", args, status, stderr)
		}
		assertContains(t, stdout, "ultraplan sprint")
	}
}

func TestSprintStatusRefreshesStateAndRendersDeterministically(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, "# Requirements\n", "requirements.md")
	writeFixtureFileContent(t, base, "# Sprint Index\n\nNo reasoning templates selected.\n", "sprint-index.md")
	writeFixtureFileContent(t, base, "# Handbook\n", "technical-handbook.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "status"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Project: proj\n")
	assertContains(t, stdout, "Sprint: 01-alpha\n")
	assertContains(t, stdout, "Flow state: projects/proj/sprints/01-alpha/flow-state.json\n")
	assertInOrder(t, stdout, "  requirements: complete", "  sprint-index: complete")
	assertInOrder(t, stdout, "  sprint-index: complete", "  technical-handbook: complete")
	assertInOrder(t, stdout, "  technical-handbook: complete", "  area-reasoning: skipped")
	assertInOrder(t, stdout, "  area-reasoning: skipped", "  reasoning: ready")
	assertInOrder(t, stdout, "  reasoning: ready", "  plan: missing")
	if strings.Contains(stdout+stderr, "\x1b[") {
		t.Fatalf("unexpected ANSI escape sequence")
	}
	if _, err := os.Stat(filepath.Join(base, "flow-state.json")); err != nil {
		t.Fatalf("flow state not written: %v", err)
	}
}

func TestSprintStatusErrorsAndInvalidFlowStateExitFive(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "api", "01-alpha")
	writeCommandSprintProject(t, dir, "api", "02-alpha")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "api", "0", "status"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("ambiguous status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `ambiguous sprint reference "0"`)

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "missing", "01", "status"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("missing project status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `project reference "missing" not found`)

	base := filepath.Join(dir, "projects", "api", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, `{"schemaVersion":1}`, "flow-state.json")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "api", "01-alpha", "status"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("invalid state status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "flow state malformed")
	content, err := os.ReadFile(filepath.Join(base, "flow-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"schemaVersion":1}` {
		t.Fatalf("invalid state was overwritten: %s", content)
	}
}

func TestSprintMalformedArgumentsUseUsageExit(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"sprint", "proj", "status"})
	if status != ExitUsage || stdout != "" {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "expected '<project> <sprint> status'")
}

func TestSprintValidatePromptAndDryRunCommands(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, "# Requirements\n\nSelect stage.\n", "requirements.md")
	writeFixtureFileContent(t, base, commandValidSprintIndex(), "sprint-index.md")
	writeFixtureFileContent(t, base, commandValidTechnicalHandbook(), "technical-handbook.md")
	writeFixtureFileContent(t, filepath.Join(dir, "projects", "proj"), commandProjectIndex(), "project-index.md")
	writeFixtureFileContent(t, dir, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFixtureFileContent(t, dir, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "sprint-index"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "technical-handbook"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("handbook validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "technical-handbook.md")
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "sprint-index"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Sprint Index")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-sprint-index.md`")
	assertContains(t, stdout, "Injected Sprint Index Template:")
	assertContains(t, stdout, "Source: builtin:templates/sprint-index.md")
	assertContains(t, stdout, "Do not mutate")
	if strings.Contains(stdout+stderr, "\x1b[") || strings.Contains(stdout, dir) {
		t.Fatalf("unsafe prompt output stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "technical-handbook"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("handbook prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Technical Handbook")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-technical-handbook.md`")
	assertContains(t, stdout, "Selected evidence:")
	if strings.Contains(stdout+stderr, "\x1b[") || strings.Contains(stdout, dir) {
		t.Fatalf("unsafe handbook prompt output stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "sprint-index", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("flow dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Dry run: true")
	if _, err := os.Stat(filepath.Join(base, "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote state: %v", err)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "technical-handbook", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("handbook flow dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Flow target: technical-handbook")
	assertContains(t, stdout, "Dry run: true")

	writeFixtureFileContent(t, base, commandValidAreaReasoning(), "reasoning", "architecture.md")
	writeFixtureFileContent(t, base, commandValidReasoning(), "reasoning.md")
	writeFixtureFileContent(t, base, commandValidPlan(), "plan.md")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "area-reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("area validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("reasoning validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "reasoning.md")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "plan"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("plan validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "plan.md")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "area-reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("area prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Area Reasoning")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-area-reasoning.md`")
	assertContains(t, stdout, "## Area Decisions")
	assertContains(t, stdout, "## Trade-Offs")
	assertContains(t, stdout, "## Evidence")
	assertContains(t, stdout, "## Risks")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("reasoning prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Sprint Reasoning")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-sprint-reasoning.md`")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "plan"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("plan prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Sprint Planning - Evidence-Grounded Implementation Plan")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/plan-sprint.md`")
	assertContains(t, stdout, "Do not execute implementation tasks")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "reasoning", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("reasoning dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Flow target: reasoning")
	assertContains(t, stdout, "Dry run: true")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "plan", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("plan dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Flow target: plan")
	assertContains(t, stdout, "Dry run: true")
}

func TestSprintFlowNonDryRunUsesConfiguredRuntime(t *testing.T) {
	dir := initializedWorkspace(t)
	writeFixtureFileContent(t, dir, `version: 1
runtime:
  default: opencode
models:
  default: minimax-coding-plan/MiniMax-M3
  primary: minimax-coding-plan/MiniMax-M3
  backup: minimax-coding-plan/MiniMax-M3
execution:
  default_variant: high
  default_parallel: 1
  default_timeout: 12m
  default_retries: 1
planning:
  requirements_model: openai/gpt-5.5
  requirements_variant: high
  sprint_index_model: openai/gpt-5.5
  sprint_index_variant: high
  reasoning_model: openai/gpt-5.5
  reasoning_variant: high
  plan_model: openai/gpt-5.5
  plan_variant: high
logging:
  format: text
  level: info
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
`, "ultraplan.yml")
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, "# Requirements\n\nSelect stage.\n", "requirements.md")
	writeFixtureFileContent(t, base, commandValidSprintIndex(), "sprint-index.md")
	writeFixtureFileContent(t, filepath.Join(dir, "projects", "proj"), commandProjectIndex(), "project-index.md")
	writeFixtureFileContent(t, dir, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFixtureFileContent(t, dir, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")

	fake := &sprintCommandRuntime{}
	restore := stubSprintRuntimeFactory(fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "sprint-index"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("flow status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Result: sprint-index complete")
	if fake.calls != 2 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if fake.request.Provider == "" || fake.request.Model == "" {
		t.Fatalf("runtime request did not include config: %+v", fake.request)
	}
	if fake.request.Provider != "openai" || fake.request.Model != "gpt-5.5" {
		t.Fatalf("planning model override was not used: %+v", fake.request)
	}
	if fake.request.Metadata["reasoning_effort"] != "high" {
		t.Fatalf("planning variant metadata was not used: %+v", fake.request.Metadata)
	}
	if fake.request.Metadata["stage"] != "sprint-index" {
		t.Fatalf("runtime metadata = %+v", fake.request.Metadata)
	}
	assertContains(t, fake.request.Prompt, "# Create Sprint Index")
	assertContains(t, fake.request.Prompt, "Prompt source: `builtin:prompts/create-sprint-index.md`")

	writeFixtureFileContent(t, base, commandValidTechnicalHandbook(), "technical-handbook.md")
	writeFixtureFileContent(t, base, commandValidAreaReasoning(), "reasoning", "architecture.md")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("reasoning flow status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Result: reasoning complete")
	if fake.calls != 7 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if fake.request.Metadata["stage"] != "reasoning" {
		t.Fatalf("runtime metadata = %+v", fake.request.Metadata)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "plan"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("plan flow status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Result: plan complete")
	if fake.calls != 13 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if fake.request.Metadata["stage"] != "plan" {
		t.Fatalf("runtime metadata = %+v", fake.request.Metadata)
	}
}

func TestSprintValidateFailuresAndUnsupportedStages(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, "# Requirements\n\nSelect stage.\n", "requirements.md")
	writeFixtureFileContent(t, base, "# Sprint Index\n\nTODO\n", "sprint-index.md")
	writeFixtureFileContent(t, filepath.Join(dir, "projects", "proj"), commandProjectIndex(), "project-index.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "sprint-index"})
	if status != ExitValidation {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: failed")
	assertContains(t, stderr, "sprint-index validation failed")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "review"})
	if status != ExitUsage || stdout != "" {
		t.Fatalf("unsupported status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stderr, "unsupported flow target")
}

func writeCommandSprintProject(t *testing.T, root, projectName, sprintSlug string) {
	t.Helper()
	base := filepath.Join(root, "projects", projectName)
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints", sprintSlug)
	writeFixtureFileContent(t, base, "# PRD\n", "docs", "PRD.md")
	writeFixtureFileContent(t, base, "# Roadmap\n", "roadmap.md")
	writeFixtureFileContent(t, base, "# Project Index\n", "project-index.md")
}

func commandProjectIndex() string {
	return `# Project Index

## Active Contract Pool

| Contract | Path | Applies To |
|---|---|---|
| Architecture | .ultra/system/contracts/core/architecture.md | All sprints |

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

func commandValidSprintIndex() string {
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

func commandValidRequirements() string {
	return `# Sprint Requirements: 01-alpha

> Project: proj
> Sprint: 01-alpha

## Sprint Goal

Select sprint context for the next planning stage.

## Required Outputs

| Output | Path | Description |
|---|---|---|
| Sprint index | projects/proj/sprints/01-alpha/sprint-index.md | Selected context |

## Acceptance Criteria

- [ ] Requirements are specific.

## Non-Goals

- Smoke investigation.

## Constraints

- Use workspace-relative paths.

## Dependencies

| Prior Sprint / Output | Required For | Notes |
|---|---|---|
| Project index | Planning | Must validate |

## Review Expectations

| What | How Verified |
|---|---|
| Requirements | Validation |
`
}

type sprintCommandRuntime struct {
	calls   int
	request runtimepkg.Request
}

func (f *sprintCommandRuntime) StartRun(_ context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	f.calls++
	f.request = req
	if req.Metadata["stage"] == string(sprint.StageRequirements) {
		path := filepath.Join(req.WorkDir, "projects", req.Metadata["project"], "sprints", req.Metadata["sprint"], "requirements.md")
		if err := os.WriteFile(path, []byte(commandValidRequirements()), 0o644); err != nil {
			return runtimepkg.Result{}, err
		}
	}
	if req.Metadata["stage"] == string(sprint.StageReasoning) {
		path := filepath.Join(req.WorkDir, "projects", req.Metadata["project"], "sprints", req.Metadata["sprint"], "reasoning.md")
		if err := os.WriteFile(path, []byte(commandValidReasoning()), 0o644); err != nil {
			return runtimepkg.Result{}, err
		}
	}
	if req.Metadata["stage"] == string(sprint.StagePlan) {
		path := filepath.Join(req.WorkDir, "projects", req.Metadata["project"], "sprints", req.Metadata["sprint"], "plan.md")
		if err := os.WriteFile(path, []byte(commandValidPlan()), 0o644); err != nil {
			return runtimepkg.Result{}, err
		}
	}
	return runtimepkg.Result{RunID: "sprint-run", Status: "completed"}, nil
}

func stubSprintRuntimeFactory(rt *sprintCommandRuntime) func() {
	orig := sprintRuntimeFactory
	sprintRuntimeFactory = func(config.Config) (sprint.Runtime, error) {
		return rt, nil
	}
	return func() { sprintRuntimeFactory = orig }
}

func commandValidTechnicalHandbook() string {
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

func commandValidAreaReasoning() string {
	return `# Architecture Reasoning

## Area Decisions

- Architecture uses .ultra/system/reasoning/architecture_reasoning_template.md.

## Trade-Offs

- Local validation keeps ownership clear.

## Evidence

- .ultra/studies/go-cli-study/reports/final/01-project-structure.md

## Risks

- Structural validation is limited.
`
}

func commandValidReasoning() string {
	return `# Sprint Reasoning

## Sprint Purpose

Implement reasoning.

## Selected Context And Pre-Reasoning Artifacts

- requirements.md

## Area-Specific Reasoning Inputs

- Architecture: projects/proj/sprints/01-alpha/reasoning/architecture.md

## Decisions

- Keep behavior in internal/sprint.

## Expected Evidence

- go test ./...

## Assumptions And Risks

- Structural validation has limits.

## Implementation Constraints

- Do not generate or validate plan.md.
`
}

func commandValidPlan() string {
	return `# Sprint Plan

## Reasoning Source

- Source: projects/proj/sprints/01-alpha/reasoning.md

## Sprint Status

- Status: not started

## Decisions To Execute

| Decision | Source |
|---|---|
| Keep behavior in internal/sprint | reasoning.md |

## Requirements / Contracts To Satisfy

| Contract | Evidence |
|---|---|
| AC-01 | go test ./... |

## Tasks

- [ ] Task 1: Add plan behavior for Decision 1 / AC-01
  > Executes: Decision 1, AC-01
  - [ ] Verification expectation: go test ./...

## Evidence Checklist

- [ ] Command tests pass.

## Risks And Blockers

| Risk | Mitigation |
|---|---|
| Structural validation | Keep checks focused. |

## Execution Log

| Step | Evidence |
|---|---|
| pending | pending |

## Completion Criteria

- [ ] Tests pass.
`
}
