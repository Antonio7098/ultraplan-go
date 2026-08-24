package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaterialiseAllStageSkills(t *testing.T) {
	root := t.TempDir()
	plan, err := PlanSkills(root, "all", SkillsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) == 0 {
		t.Fatal("expected planned skill operations")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote .agents: %v", err)
	}

	if _, err := MaterialiseSkills(root, "all", SkillsOptions{}); err != nil {
		t.Fatal(err)
	}
	skills := StageSkills()
	if len(skills) != 11 {
		t.Fatalf("stage skill count = %d, want 11", len(skills))
	}
	for _, skill := range skills {
		base := filepath.Join(root, ".agents", "skills", skill.Name)
		content, err := os.ReadFile(filepath.Join(base, "SKILL.md"))
		if err != nil {
			t.Fatalf("read %s: %v", skill.Name, err)
		}
		body := string(content)
		wants := []string{
			"name: " + skill.Name,
			"do not stop at a proposal",
			"status --json",
			"Canonical stage prompt",
			strings.TrimSpace(skill.Prompt),
		}
		if skill.Stage == "reconcile" {
			wants = append(wants, "A stale review fingerprint is context for reconciliation, not a prerequisite failure")
		} else {
			wants = append(wants, "ask whether to fill them")
		}
		for _, want := range wants {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing %q", skill.Name, want)
			}
		}
		metadata, err := os.ReadFile(filepath.Join(base, "agents", "openai.yaml"))
		if err != nil {
			t.Fatalf("read %s metadata: %v", skill.Name, err)
		}
		if !strings.Contains(string(metadata), "allow_implicit_invocation: false") {
			t.Fatalf("%s is not manual-only", skill.Name)
		}
	}

	idempotent, err := MaterialiseSkills(root, "all", SkillsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(idempotent.Operations) != 0 {
		t.Fatalf("idempotent operations = %#v", idempotent.Operations)
	}
}

func TestMaterialiseOneStageAndPreserveCustomization(t *testing.T) {
	root := t.TempDir()
	if _, err := MaterialiseSkills(root, "reasoning", SkillsOptions{}); err != nil {
		t.Fatal(err)
	}
	reasoning := filepath.Join(root, ".agents", "skills", "ultraplan-reasoning", "SKILL.md")
	if err := os.WriteFile(reasoning, []byte("# Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err := MaterialiseSkills(root, "ultraplan-reasoning", SkillsOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Operations) != 1 || plan.Operations[0].Action != "skip" || plan.Operations[0].Path != ".agents/skills/ultraplan-reasoning/SKILL.md" {
		t.Fatalf("customized plan = %#v", plan.Operations)
	}
	content, err := os.ReadFile(reasoning)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Custom\n" {
		t.Fatal("customized skill was overwritten without force")
	}
	if _, err := MaterialiseSkills(root, "reasoning", SkillsOptions{Force: true}); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(reasoning)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "name: ultraplan-reasoning") {
		t.Fatal("force did not restore built-in skill")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents", "skills", "ultraplan-plan")); !os.IsNotExist(err) {
		t.Fatalf("single-stage materialisation wrote another skill: %v", err)
	}
}

func TestResolveStageSkillsRejectsUnknownSelection(t *testing.T) {
	if _, err := ResolveStageSkills("unknown"); err == nil || !strings.Contains(err.Error(), "technical-handbook") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewSkillResolvesSprintPathAndDelegatesFanOut(t *testing.T) {
	skills, err := ResolveStageSkills("review")
	if err != nil {
		t.Fatal(err)
	}
	body := renderStageSkill(skills[0])
	for _, want := range []string{
		"Treat a supplied sprint path as UltraPlan stage input",
		"read the matching `project-index.md`",
		"resolve its repository from `Target Implementation Directory`",
		"Do not search nested source repositories for a similarly named skill",
		"projects/ultraplan-go/sprints/30-web-foundations/",
		"The CLI owns reviewer fan-out",
		"read the generated `review.md`",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("review skill missing %q", want)
		}
	}
}

func TestReconciliationSkillCoversFindingTriageAndSmokeHarnessReadiness(t *testing.T) {
	skills, err := ResolveStageSkills("reconcile")
	if err != nil {
		t.Fatal(err)
	}
	body := renderStageSkill(skills[0])
	for _, want := range []string{
		"name: ultraplan-reconcile-review-smoke",
		"genuine sprint defect",
		"Preserve unrelated user changes",
		"current governed input fingerprint",
		"A stale review fingerprint is context for reconciliation, not a prerequisite failure",
		"continue classifying the existing review findings",
		"do not ask to rerun review or stop reconciliation merely because they differ",
		"Do not use `validate execute` as a reconciliation prerequisite after execution",
		"recompute the review artifact SHA-256",
		"explicitly authorized manual review or smoke reconciliation branches",
		"sprint smoke --dry-run --json",
		"../ultraplan-go-smoke",
		"ultraplan-smoke.json",
		"requiredCoverage",
		"notApplicable true and complete false",
		"ready true",
		"update the sprint-root smoke.md and smoke flow state as one coherent result",
		"Recompute the smoke.md SHA-256 and the input fingerprint",
		"active-attempt state, and last-complete identity",
		"Run validate smoke and sprint status --json immediately",
		"do not claim that smoke or the sprint is passing",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("reconciliation skill missing %q", want)
		}
	}
	if strings.Contains(body, "validate reconcile") {
		t.Fatal("reconciliation skill advertises a nonexistent validation stage")
	}
	byName, err := ResolveStageSkills("ultraplan-reconcile-review-smoke")
	if err != nil || len(byName) != 1 || byName[0].Stage != "reconcile" {
		t.Fatalf("resolve reconciliation skill by name = %+v, %v", byName, err)
	}
}

func TestOnlyReviewAndCodeContextDelegateStageExecutionToCLI(t *testing.T) {
	for _, skill := range StageSkills() {
		body := renderStageSkill(skill)
		ownsStageWork := strings.Contains(body, "The invoking agent owns the actual stage work") ||
			(skill.Stage == "execute" && strings.Contains(body, "Act as the execution agent and perform the entire stage manually")) ||
			(skill.Stage == "code-context" && strings.Contains(body, "manual-only skill deliberately delegates"))
		if !ownsStageWork {
			t.Fatalf("%s skill is missing the agent-owned execution contract", skill.Name)
		}
		if skill.Stage == "review" {
			if !strings.Contains(body, "ultraplan sprint <project> <sprint> review") {
				t.Fatal("review skill does not invoke the governed CLI review")
			}
			continue
		}
		if skill.Stage == "code-context" {
			for _, want := range []string{`PROJECT="<project>"`, `SPRINT="<sprint>"`, `ultraplan sprint "$PROJECT" "$SPRINT" flow --to code-context`, "allow_implicit_invocation: false"} {
				if want == "allow_implicit_invocation: false" {
					continue
				}
				if !strings.Contains(body, want) {
					t.Fatalf("code-context skill missing canonical delegation %q", want)
				}
			}
			continue
		}
		for _, forbidden := range []string{
			"    ultraplan sprint <project> <sprint> execute --resume",
			"    ultraplan sprint <project> <sprint> smoke --yes",
			"    ultraplan sprint <project> <sprint> flow --to " + skill.Stage,
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s delegates stage execution through forbidden CLI instruction %q", skill.Name, forbidden)
			}
		}
	}
}

func TestExecuteSkillUsesCLIOnlyForStateAndPrompt(t *testing.T) {
	skills, err := ResolveStageSkills("execute")
	if err != nil {
		t.Fatal(err)
	}
	body := renderStageSkill(skills[0])
	for _, want := range []string{
		"Act as the execution agent",
		"perform the entire stage manually",
		"Read `<sprint>/.workspace.json`",
		"use its absolute `path` as the implementation target",
		"Git reports it as a worktree of the recorded `sourceRoot` on the recorded `branch`",
		"Never implement in `Target Implementation Directory`",
		"Do not guess a worktree or fall back to another checkout",
		"This worktree rule overrides target-directory wording",
		"Use the UltraPlan CLI only for",
		"sprint <project> <sprint> prompt execute",
		"run the plan's required checks directly",
		"review --dry-run --json",
		"`result.execution_status` to be `ready`",
		"repeat the review dry-run until it is ready",
		"Do not launch the actual review from this skill",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("execute skill missing %q", want)
		}
	}
	for _, forbidden := range []string{
		"dry-run previews, status inspection, validation",
		"sprint <project> <sprint> validate execute",
		"sprint <project> <sprint> execute --dry-run",
		"sprint <project> <sprint> execute --resume",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("execute skill permits forbidden CLI operation %q", forbidden)
		}
	}
}
