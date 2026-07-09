package sprint

import (
	"strings"
	"testing"
	"time"
)

func TestExtractExecutePlanTasks(t *testing.T) {
	manifest := executePlanManifest()
	tasks, findings := ExtractExecutePlanTasks(validPlan(), manifest)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %+v", tasks)
	}
	task := tasks[0]
	if task.ID == "" || task.Name != "Task 1: Add plan behavior for Decision 1 / AC-01" || task.PlanLine == 0 {
		t.Fatalf("task = %+v", task)
	}
	if strings.Join(task.Decisions, ",") != "Decision 1" || strings.Join(task.Requirements, ",") != "AC-01" {
		t.Fatalf("traceability = %+v %+v", task.Decisions, task.Requirements)
	}
	if len(task.Evidence) == 0 || !strings.Contains(task.Evidence[0], "go test ./...") {
		t.Fatalf("evidence = %+v", task.Evidence)
	}
}

func TestExtractExecutePlanTaskIDsAreStableAndChangeWithIdentity(t *testing.T) {
	manifest := executePlanManifest()
	first, findings := ExtractExecutePlanTasks(validPlan(), manifest)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	spaced := strings.ReplaceAll(validPlan(), "Task 1: Add plan behavior", "Task   1:   Add   plan   behavior")
	second, findings := ExtractExecutePlanTasks(spaced, manifest)
	if len(findings) != 0 {
		t.Fatalf("spaced findings = %+v", findings)
	}
	if first[0].ID != second[0].ID {
		t.Fatalf("equivalent task changed id: %s != %s", first[0].ID, second[0].ID)
	}
	changedPlan := strings.Replace(validPlan(), "- [ ] Task 1: Add plan behavior for Decision 1 / AC-01", "- [ ] Task 1: Add plan behavior for Decision 1 / AC-02", 1)
	changed, findings := ExtractExecutePlanTasks(changedPlan, manifest)
	if len(findings) != 0 {
		t.Fatalf("changed findings = %+v", findings)
	}
	if first[0].ID == changed[0].ID {
		t.Fatalf("identity change did not change id")
	}
}

func TestExtractExecutePlanTasksRejectsUnsupportedAmbiguousAndDuplicateTasks(t *testing.T) {
	manifest := executePlanManifest()
	cases := map[string]string{
		"no executable tasks": strings.ReplaceAll(validPlan(), "- [ ] Task 1:", "- [x] Task 1:"),
		"unsupported syntax":  strings.ReplaceAll(validPlan(), "- [ ] Task 1:", "- [ ] Work 1:"),
		"ambiguous nested":    strings.Replace(validPlan(), "## Tasks\n\n", "## Tasks\n\n  - [ ] Floating nested item\n", 1),
		"duplicate id":        strings.Replace(validPlan(), "## Evidence Checklist", "- [ ] Task 1: Add plan behavior for Decision 1 / AC-01\n  > Executes: Decision 1, AC-01\n  - [ ] Verification expectation: go test ./...\n\n## Evidence Checklist", 1),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			if _, findings := ExtractExecutePlanTasks(content, manifest); len(findings) == 0 {
				t.Fatalf("expected findings")
			}
		})
	}
}

func TestValidateExecuteRequiresValidPlanAndExtractableTasks(t *testing.T) {
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

	result, err := NewService(root).ValidateExecute("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid() {
		t.Fatalf("findings = %+v", result.Findings)
	}

	writeFileContent(t, sp.Path, strings.ReplaceAll(validPlan(), "- [ ] Task 1:", "- [x] Task 1:"), "plan.md")
	result, err = NewService(root).ValidateExecute("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid() {
		t.Fatalf("expected invalid execute prerequisites")
	}
}

func TestExecuteTasksToRecords(t *testing.T) {
	now := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	tasks, findings := ExtractExecutePlanTasks(validPlan(), executePlanManifest())
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	records := ExecuteTasksToRecords(tasks, func() time.Time { return now })
	if len(records) != 1 || records[0].Status != ExecuteTaskPending || !records[0].CreatedAt.Equal(now) {
		t.Fatalf("records = %+v", records)
	}
}

func executePlanManifest() PlanManifest {
	return PlanManifest{
		OutputPath:    "projects/proj/sprints/01-alpha/plan.md",
		ReasoningPath: "projects/proj/sprints/01-alpha/reasoning.md",
		DecisionNames: []string{"Keep Plan Behavior In Sprint"},
	}
}
