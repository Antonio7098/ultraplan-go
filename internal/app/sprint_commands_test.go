package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	assertContains(t, stdout, "Generate sprint-index.md")
	assertContains(t, stdout, "Do not mutate")
	if strings.Contains(stdout+stderr, "\x1b[") || strings.Contains(stdout, dir) {
		t.Fatalf("unsafe prompt output stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "technical-handbook"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("handbook prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Generate technical-handbook.md")
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
