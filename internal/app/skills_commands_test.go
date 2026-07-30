package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSkillsMaterialiseDryRunOneAndAll(t *testing.T) {
	dir := initializedWorkspace(t)
	stdout, stderr, status := runForTest([]string{"skills", "materialise", "reasoning", "--path", dir, "--dry-run"})
	if status != ExitOK {
		t.Fatalf("dry-run status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Selection: reasoning")
	assertContains(t, stdout, "would create file .agents/skills/ultraplan-reasoning/SKILL.md")
	if _, err := os.Stat(filepath.Join(dir, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote .agents: %v", err)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "skills", "materialize"})
	if status != ExitOK {
		t.Fatalf("materialize alias status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Selection: all")
	for _, name := range []string{"requirements", "sprint-index", "technical-handbook", "area-reasoning", "reasoning", "plan", "execute", "review", "smoke"} {
		path := filepath.Join(dir, ".agents", "skills", "ultraplan-"+name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", path, err)
		}
	}
}

func TestSkillsMaterialisePreservesCustomizedFilesUnlessConfirmed(t *testing.T) {
	dir := initializedWorkspace(t)
	_, stderr, status := runForTest([]string{"skills", "materialise", "plan", "--path", dir})
	if status != ExitOK {
		t.Fatalf("initial materialise status = %d stderr = %q", status, stderr)
	}
	custom := filepath.Join(dir, ".agents", "skills", "ultraplan-plan", "SKILL.md")
	if err := os.WriteFile(custom, []byte("# Custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, status := runForTestWithInput([]string{"skills", "materialise", "plan", "--path", dir}, nil, "no\n")
	if status != ExitOK {
		t.Fatalf("preserve status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Keeping customized stage skills")
	content, err := os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "# Custom\n" {
		t.Fatal("custom skill was overwritten")
	}

	stdout, stderr, status = runForTestWithInput([]string{"skills", "materialise", "plan", "--path", dir}, nil, "yes\n")
	if status != ExitOK {
		t.Fatalf("confirm status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "Overwriting customized stage skills")
	content, err = os.ReadFile(custom)
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, string(content), "name: ultraplan-plan")
}
