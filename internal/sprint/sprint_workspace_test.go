package sprint

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateSprintWorkspaceFreezesBaselineAndReusesWorktree(t *testing.T) {
	source := gitFixture(t)
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "Project One", "01 Parallel Work")
	index := projectIndexForTarget(source)
	service := NewService(root).WithClock(func() time.Time { return time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC) })

	target, findings := service.resolveSprintTarget(sp, index, true)
	if len(findings) > 0 {
		t.Fatalf("create findings = %+v", findings)
	}
	record, err := loadSprintWorkspace(sp)
	if err != nil {
		t.Fatal(err)
	}
	if target.Path != record.Path || target.Source != ".workspace.json" || record.Branch != "ultraplan/project-one/01-parallel-work" {
		t.Fatalf("target=%+v record=%+v", target, record)
	}
	if got := mustGitOutput(t, record.Path, "rev-parse", "HEAD"); got != record.Baseline {
		t.Fatalf("worktree HEAD = %q, baseline = %q", got, record.Baseline)
	}
	if err := os.WriteFile(filepath.Join(record.Path, "sprint-change.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	again, findings := service.resolveSprintTarget(sp, index, true)
	if len(findings) > 0 || again.Path != record.Path {
		t.Fatalf("reuse target=%+v findings=%+v", again, findings)
	}
}

func TestCreateSprintWorkspaceRejectsDirtySource(t *testing.T) {
	source := gitFixture(t)
	if err := os.WriteFile(filepath.Join(source, "uncommitted.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	_, findings := NewService(root).resolveSprintTarget(sp, projectIndexForTarget(source), true)
	if len(findings) != 1 || !strings.Contains(findings[0].Cause, "uncommitted changes") {
		t.Fatalf("findings = %+v", findings)
	}
	if _, err := os.Stat(sprintWorkspacePath(sp)); !os.IsNotExist(err) {
		t.Fatalf("workspace record created after rejection: %v", err)
	}
}

func TestCompletedCodeContextMaterializesWorkspaceWithoutRerun(t *testing.T) {
	source := gitFixture(t)
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFileContent(t, filepath.Join(root, "projects", "proj"), projectIndexForTarget(source), "project-index.md")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	writeFileContent(t, sp.Path, validCodeContext(), "code-context.md")
	state := NewFlowState(sp, flowCodeContextSuccessStages(sp, time.Now().UTC()), time.Now().UTC())
	if err := SaveFlowState(root, sp, state); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(sp.Path, "code-context.md"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewService(root).Flow(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext})
	if err != nil || result.Message != "code-context already complete" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := loadSprintWorkspace(sp); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(sp.Path, "code-context.md"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("completed code-context changed: err=%v", err)
	}
}

func gitFixture(t *testing.T) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, source, "init", "-b", "main")
	runGitTest(t, source, "config", "user.name", "UltraPlan Test")
	runGitTest(t, source, "config", "user.email", "test@ultraplan.invalid")
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, source, "add", "README.md")
	runGitTest(t, source, "commit", "-m", "baseline")
	t.Cleanup(func() {
		output, err := exec.Command("git", "-C", source, "worktree", "list", "--porcelain").Output()
		if err != nil {
			return
		}
		for _, block := range strings.Split(string(output), "\n\n") {
			line := strings.Split(block, "\n")[0]
			path := strings.TrimPrefix(line, "worktree ")
			if path != "" && filepath.Clean(path) != filepath.Clean(source) {
				_ = exec.Command("git", "-C", source, "worktree", "remove", "--force", path).Run()
			}
		}
	})
	return source
}

func projectIndexForTarget(target string) string {
	return "# Project Index\n\n## Project Scope\n\n- **Target Implementation Directory:** " + target + "\n"
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
}

func mustGitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %s: %v", strings.Join(args, " "), output, err)
	}
	return strings.TrimSpace(string(output))
}
