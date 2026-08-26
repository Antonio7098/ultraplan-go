package sprint

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const reviewPatchPath = "target/.ultraplan/changes.patch"

func buildReviewPatch(sp Sprint, target string, changedPaths []string) (string, bool, error) {
	record, err := loadSprintWorkspace(sp)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load sprint workspace for review patch: %w", err)
	}
	if filepath.Clean(record.Path) != filepath.Clean(target) {
		return "", false, fmt.Errorf("review target %q does not match sprint worktree %q", target, record.Path)
	}
	paths := uniqueSorted(changedPaths)
	if len(paths) == 0 {
		return "", false, nil
	}
	args := append([]string{"-C", target, "diff", "--binary", "--no-ext-diff", "--full-index", record.Baseline, "--"}, paths...)
	tracked, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("render sprint review patch: %s: %w", strings.TrimSpace(string(tracked)), err)
	}
	untrackedArgs := append([]string{"-C", target, "ls-files", "--others", "--exclude-standard", "-z", "--"}, paths...)
	untrackedOutput, err := exec.Command("git", untrackedArgs...).CombinedOutput()
	if err != nil {
		return "", false, fmt.Errorf("identify untracked review paths: %s: %w", strings.TrimSpace(string(untrackedOutput)), err)
	}
	var patch strings.Builder
	patch.Write(tracked)
	for _, path := range strings.Split(string(untrackedOutput), "\x00") {
		if path == "" {
			continue
		}
		cmd := exec.Command("git", "-C", target, "diff", "--no-index", "--binary", "--no-ext-diff", "--full-index", "--", "/dev/null", path)
		output, diffErr := cmd.CombinedOutput()
		var exitErr *exec.ExitError
		if diffErr != nil && (!errors.As(diffErr, &exitErr) || exitErr.ExitCode() != 1) {
			return "", false, fmt.Errorf("render untracked review patch %q: %s: %w", path, strings.TrimSpace(string(output)), diffErr)
		}
		patch.Write(output)
	}
	if patch.Len() == 0 {
		return "", false, fmt.Errorf("sprint run state names changed paths but baseline %s has no corresponding diff", record.Baseline)
	}
	return patch.String(), true, nil
}
