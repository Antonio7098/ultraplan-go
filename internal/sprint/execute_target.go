package sprint

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const sprintWorkspaceSchemaVersion = 1

type SprintWorkspace struct {
	SchemaVersion int       `json:"schemaVersion"`
	SourceRoot    string    `json:"sourceRoot"`
	Path          string    `json:"path"`
	Branch        string    `json:"branch"`
	Baseline      string    `json:"baseline"`
	CreatedAt     time.Time `json:"createdAt"`
}

func sprintWorkspacePath(sp Sprint) string { return filepath.Join(sp.Path, ".workspace.json") }

func ResolveExecuteTarget(projectIndexContent string) (ExecuteTargetRef, []ValidationFinding) {
	target := extractTargetImplementationDirectory(projectIndexContent)
	if target == "" {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Project Scope", "Target Implementation Directory", "", "missing target implementation directory", "project-index.md does not declare a target implementation directory", "Set Project Scope / Target Implementation Directory to the approved implementation repository.")}
	}
	clean := filepath.Clean(target)
	if !filepath.IsAbs(clean) {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Project Scope", "Target Implementation Directory", target, "target path must be absolute", "execute requires an explicit absolute target repository path", "Use the approved target implementation directory.")}
	}
	info, err := os.Stat(clean)
	if err != nil {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Project Scope", "Target Implementation Directory", target, "target root unavailable", err.Error(), "Create or restore the approved target repository before execute.")}
	}
	if !info.IsDir() {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Project Scope", "Target Implementation Directory", target, "target root is not a directory", "path exists but is not a directory", "Use the approved target repository directory.")}
	}
	return ExecuteTargetRef{Path: clean, Source: "project-index.md"}, nil
}

func (s Service) resolveSprintTarget(sp Sprint, projectIndex string, create bool) (ExecuteTargetRef, []ValidationFinding) {
	if s.codeContextTarget != nil {
		return s.codeContextTarget(projectIndex)
	}
	source, findings := ResolveExecuteTarget(projectIndex)
	if len(findings) > 0 {
		return ExecuteTargetRef{}, findings
	}
	record, err := loadSprintWorkspace(sp)
	if err == nil {
		if validateErr := validateSprintWorkspace(record, source.Path); validateErr != nil {
			return ExecuteTargetRef{}, []ValidationFinding{finding("Sprint Workspace", "workspace", workspace.Rel(s.root, sprintWorkspacePath(sp)), "invalid sprint workspace", validateErr.Error(), "Restore the recorded worktree or explicitly recreate the sprint workspace.")}
		}
		return ExecuteTargetRef{Path: record.Path, Source: ".workspace.json"}, nil
	}
	if !os.IsNotExist(err) {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Sprint Workspace", "workspace", workspace.Rel(s.root, sprintWorkspacePath(sp)), "unreadable sprint workspace", err.Error(), "Repair or remove the malformed sprint workspace record.")}
	}
	if !create {
		return source, nil
	}
	record, err = createSprintWorkspace(sp, source.Path, s.now().UTC())
	if err != nil {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Sprint Workspace", "workspace", source.Path, "cannot create sprint workspace", err.Error(), "Commit or stash source changes, remove any conflicting branch or worktree, then retry.")}
	}
	return ExecuteTargetRef{Path: record.Path, Source: ".workspace.json"}, nil
}

func loadSprintWorkspace(sp Sprint) (SprintWorkspace, error) {
	data, err := os.ReadFile(sprintWorkspacePath(sp))
	if err != nil {
		return SprintWorkspace{}, err
	}
	var record SprintWorkspace
	if err := json.Unmarshal(data, &record); err != nil {
		return SprintWorkspace{}, fmt.Errorf("decode sprint workspace: %w", err)
	}
	if record.SchemaVersion != sprintWorkspaceSchemaVersion || record.SourceRoot == "" || record.Path == "" || record.Branch == "" || record.Baseline == "" {
		return SprintWorkspace{}, fmt.Errorf("invalid sprint workspace record")
	}
	return record, nil
}

func validateSprintWorkspace(record SprintWorkspace, source string) error {
	if filepath.Clean(record.SourceRoot) != filepath.Clean(source) {
		return fmt.Errorf("project target changed from %q to %q", record.SourceRoot, source)
	}
	info, err := os.Stat(record.Path)
	if err != nil {
		return fmt.Errorf("worktree unavailable: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path is not a directory")
	}
	sourceCommon, err := gitCommonDirectory(source)
	if err != nil {
		return fmt.Errorf("source Git metadata unavailable: %w", err)
	}
	workspaceCommon, err := gitCommonDirectory(record.Path)
	if err != nil || workspaceCommon != sourceCommon {
		return fmt.Errorf("worktree belongs to a different Git repository")
	}
	branch, err := gitOutput(record.Path, "branch", "--show-current")
	if err != nil || strings.TrimSpace(branch) != record.Branch {
		return fmt.Errorf("worktree is not on recorded branch %q", record.Branch)
	}
	return nil
}

func gitCommonDirectory(dir string) (string, error) {
	common, err := gitOutput(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	return filepath.Abs(filepath.Clean(common))
}

func createSprintWorkspace(sp Sprint, source string, now time.Time) (SprintWorkspace, error) {
	root, err := gitOutput(source, "rev-parse", "--show-toplevel")
	if err != nil {
		return SprintWorkspace{}, fmt.Errorf("target is not a Git worktree: %w", err)
	}
	root = filepath.Clean(root)
	if filepath.Clean(source) != root {
		return SprintWorkspace{}, fmt.Errorf("target must be the Git worktree root %q", root)
	}
	dirty, err := gitOutput(source, "status", "--porcelain", "--untracked-files=normal")
	if err != nil {
		return SprintWorkspace{}, fmt.Errorf("inspect target status: %w", err)
	}
	if strings.TrimSpace(dirty) != "" {
		return SprintWorkspace{}, fmt.Errorf("target has uncommitted changes")
	}
	baseline, err := gitOutput(source, "rev-parse", "HEAD")
	if err != nil {
		return SprintWorkspace{}, fmt.Errorf("resolve target HEAD: %w", err)
	}
	branch := "ultraplan/" + safeGitComponent(sp.Project) + "/" + safeGitComponent(sp.Slug)
	parent := filepath.Join(filepath.Dir(root), "."+filepath.Base(root)+"-ultraplan-worktrees")
	path := filepath.Join(parent, safeGitComponent(sp.Project), safeGitComponent(sp.Slug))
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return SprintWorkspace{}, fmt.Errorf("worktree path already exists: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return SprintWorkspace{}, fmt.Errorf("create worktree parent: %w", err)
	}
	cmd := exec.Command("git", "-C", source, "worktree", "add", "-b", branch, path, strings.TrimSpace(baseline))
	if output, err := cmd.CombinedOutput(); err != nil {
		return SprintWorkspace{}, fmt.Errorf("git worktree add: %s: %w", strings.TrimSpace(string(output)), err)
	}
	record := SprintWorkspace{SchemaVersion: sprintWorkspaceSchemaVersion, SourceRoot: root, Path: path, Branch: branch, Baseline: strings.TrimSpace(baseline), CreatedAt: now}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return SprintWorkspace{}, err
	}
	data = append(data, '\n')
	if err := atomicWriteFile(sprintWorkspacePath(sp), data); err != nil {
		_ = exec.Command("git", "-C", source, "worktree", "remove", path).Run()
		_ = exec.Command("git", "-C", source, "branch", "-D", branch).Run()
		return SprintWorkspace{}, fmt.Errorf("persist sprint workspace: %w", err)
	}
	return record, nil
}

func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return strings.TrimSpace(string(output)), nil
}

func safeGitComponent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9._-]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, ".-")
	if value == "" {
		return "sprint"
	}
	return value
}

func ValidateExecuteWorkdir(target ExecuteTargetRef, workdir string) error {
	if target.Path == "" {
		return fmt.Errorf("missing execute target")
	}
	if workdir == "" {
		return fmt.Errorf("missing execute workdir")
	}
	targetPath := filepath.Clean(target.Path)
	workPath := filepath.Clean(workdir)
	if !filepath.IsAbs(workPath) {
		return fmt.Errorf("execute workdir %q must be absolute", workdir)
	}
	if !inside(targetPath, workPath) {
		return fmt.Errorf("execute workdir %q escapes approved target %q", workdir, target.Path)
	}
	return nil
}

func ExecuteSafetyInstructions(target ExecuteTargetRef) []string {
	return []string{
		"Work only inside approved target: " + target.Path,
		"Do not create smoke.md, smoke.json, generated review.md, issues.md, or issues.json.",
		"Do not run or request Git mutation: add, commit, push, branch, checkout, reset, PR creation, or issue tracking.",
		"Do not schedule cross-sprint work, launch a TUI, or build hosted/browser UI behavior.",
	}
}

func extractTargetImplementationDirectory(content string) string {
	re := regexp.MustCompile(`(?im)^\s*-\s+\*\*Target Implementation Directory:\*\*\s*(.+?)\s*$`)
	if match := re.FindStringSubmatch(content); len(match) == 2 {
		return strings.Trim(strings.TrimSpace(match[1]), "`")
	}
	re = regexp.MustCompile(`(?im)^\s*-\s+Target Implementation Directory:\s*(.+?)\s*$`)
	if match := re.FindStringSubmatch(content); len(match) == 2 {
		return strings.Trim(strings.TrimSpace(match[1]), "`")
	}
	return ""
}
