package study

import (
	"fmt"
	"os/exec"
)

type CloneAction struct {
	Name string
	URL  string
	Dest string
}

type CloneFailure struct {
	Action CloneAction
	Err    error
}

type ClonePartialError struct {
	Failures []CloneFailure
}

func (e ClonePartialError) Error() string {
	return fmt.Sprintf("%v: %d clone action(s) failed", ErrInitPartial, len(e.Failures))
}

func (e ClonePartialError) Unwrap() error { return ErrInitPartial }

type CloneRunner interface {
	Clone(url, dest string) error
}

type GitCloneRunner struct{}

func (GitCloneRunner) Clone(url, dest string) error {
	cmd := exec.Command("git", "clone", "--depth", "1", url, dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %w: %s", err, string(out))
	}
	return nil
}

type cloneRunResult struct {
	Cloned   []CloneAction
	Failures []CloneFailure
}

func runCloneActions(runner CloneRunner, actions []CloneAction) cloneRunResult {
	if runner == nil {
		runner = GitCloneRunner{}
	}
	var result cloneRunResult
	for _, action := range actions {
		if err := runner.Clone(action.URL, action.Dest); err != nil {
			result.Failures = append(result.Failures, CloneFailure{Action: action, Err: err})
			continue
		}
		result.Cloned = append(result.Cloned, action)
	}
	return result
}
