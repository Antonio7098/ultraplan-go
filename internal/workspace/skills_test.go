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
	if len(skills) != 9 {
		t.Fatalf("stage skill count = %d, want 9", len(skills))
	}
	for _, skill := range skills {
		base := filepath.Join(root, ".agents", "skills", skill.Name)
		content, err := os.ReadFile(filepath.Join(base, "SKILL.md"))
		if err != nil {
			t.Fatalf("read %s: %v", skill.Name, err)
		}
		body := string(content)
		for _, want := range []string{
			"name: " + skill.Name,
			"ask whether to fill them",
			"do not stop at a proposal",
			"status --json",
			"Canonical stage prompt",
			strings.TrimSpace(skill.Prompt),
		} {
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
