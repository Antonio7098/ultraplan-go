package study

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type CloneAction struct {
	Name string
	URL  string
	Dest string
}

type CloneFailure struct {
	Action    CloneAction
	Code      string
	Category  string
	Component string
	Severity  string
	Retryable bool
	Timestamp time.Time
	Err       error
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
		return fmt.Errorf("git clone failed: %w: %s", err, redactGitOutput(string(out)))
	}
	return nil
}

var credentialURLPattern = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^/\s:@]+:[^@\s/]+@`)

func redactGitOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "no git output"
	}
	output = credentialURLPattern.ReplaceAllString(output, `${1}[redacted]@`)
	const maxGitErrorOutput = 4096
	if len(output) > maxGitErrorOutput {
		return output[:maxGitErrorOutput] + "... [truncated]"
	}
	return output
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
			result.Failures = append(result.Failures, CloneFailure{
				Action:    action,
				Code:      "provider.git.clone_failed",
				Category:  "provider",
				Component: "study.init.clone",
				Severity:  "error",
				Retryable: false,
				Timestamp: time.Now().UTC(),
				Err:       err,
			})
			continue
		}
		result.Cloned = append(result.Cloned, action)
	}
	return result
}
