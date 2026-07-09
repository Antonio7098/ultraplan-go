package sprint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const ApprovedExecuteTargetPath = "/home/antonioborgerees/coding/ultraplan-go"

func ResolveExecuteTarget(projectIndexContent string) (ExecuteTargetRef, []ValidationFinding) {
	target := extractTargetImplementationDirectory(projectIndexContent)
	if target == "" {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Project Scope", "Target Implementation Directory", "", "missing target implementation directory", "project-index.md does not declare a target implementation directory", "Set Project Scope / Target Implementation Directory to the approved implementation repository.")}
	}
	clean := filepath.Clean(target)
	if !filepath.IsAbs(clean) {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Project Scope", "Target Implementation Directory", target, "target path must be absolute", "execute requires an explicit absolute target repository path", "Use the approved target implementation directory.")}
	}
	approved := filepath.Clean(ApprovedExecuteTargetPath)
	if clean != approved {
		return ExecuteTargetRef{}, []ValidationFinding{finding("Project Scope", "Target Implementation Directory", target, "unsupported execute target", fmt.Sprintf("this sprint only approves %s", approved), "Update project-index.md or defer alternate target support to a later sprint.")}
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
