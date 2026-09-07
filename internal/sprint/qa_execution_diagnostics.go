package sprint

import (
	"errors"
	"strings"
	"syscall"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

type QAFailureDiagnostic struct {
	Phase      string `json:"phase"`
	Code       string `json:"code"`
	Diagnostic string `json:"diagnostic,omitempty"`
	Retryable  bool   `json:"retryable"`
}

type QAEvidenceExecutionAttempt struct {
	TestBundleID string               `json:"test_bundle_id"`
	Number       int                  `json:"number"`
	Phase        string               `json:"phase"`
	StartedAt    time.Time            `json:"started_at"`
	CompletedAt  *time.Time           `json:"completed_at,omitempty"`
	Failure      *QAFailureDiagnostic `json:"failure,omitempty"`
	RunID        string               `json:"run_id,omitempty"`
}

type QATestEvent struct {
	Test   string `json:"test"`
	Action string `json:"action"`
}

func qaFailureDiagnostic(phase string, err error, output string) *QAFailureDiagnostic {
	if err != nil {
		output = qaUnderlyingDiagnostic(err) + "\n" + output
	}
	lower := strings.ToLower(output)
	code, retry := "unknown_failure", false
	switch {
	case errors.Is(err, syscall.ENOSPC), strings.Contains(lower, "no space left on device"):
		code, retry = "disk_space_exhausted", phase != "assertion"
	case strings.Contains(lower, "connection reset"), strings.Contains(lower, "temporary failure in name resolution"), strings.Contains(lower, "tls handshake timeout"), strings.Contains(lower, "proxyconnect tcp"), strings.Contains(lower, "network is unreachable"), strings.Contains(lower, "i/o timeout"):
		code, retry = "dependency_network_failure", phase == "dependencies" || phase == "compile"
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		code = "permission_denied"
	case strings.Contains(lower, "undefined:"), strings.Contains(lower, "syntax error"), strings.Contains(lower, "build failed"):
		code = "compile_failure"
	case strings.Contains(lower, "template") && (strings.Contains(lower, "not found") || strings.Contains(lower, "undefined") || strings.Contains(lower, "missing")):
		code, phase = "fixture_failure", "fixture"
	case phase == "workspace":
		code = "workspace_copy_failure"
	case phase == "dependencies":
		code = "dependency_preparation_failure"
	case phase == "cleanup":
		code = "cleanup_failure"
	}
	return &QAFailureDiagnostic{Phase: phase, Code: code, Retryable: retry, Diagnostic: safeReportText(config.RedactText(output))}
}

func qaTestEvents(output string) []QATestEvent {
	var events []QATestEvent
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		action := ""
		switch {
		case fields[0] == "===" && fields[1] == "RUN":
			action = "run"
		case fields[0] == "---" && fields[1] == "PASS:":
			action = "pass"
		case fields[0] == "---" && fields[1] == "FAIL:":
			action = "fail"
		case fields[0] == "---" && fields[1] == "SKIP:":
			action = "skip"
		}
		if action != "" {
			events = append(events, QATestEvent{Test: fields[2], Action: action})
		}
	}
	return events
}

func qaRetryableReason(reason string) bool {
	switch reason {
	case "disk_space_exhausted", "dependency_network_failure", "execution_interrupted", "resource_capacity_unavailable":
		return true
	}
	return false
}

type qaExecutionError struct {
	Failure *QAFailureDiagnostic
	Err     error
}

func (e *qaExecutionError) Error() string { return e.Failure.Code + ": " + e.Failure.Diagnostic }
func (e *qaExecutionError) Unwrap() error { return e.Err }

func qaUnderlyingDiagnostic(err error) string {
	var parts []string
	seen := map[string]bool{}
	var visit func(error, int)
	visit = func(current error, depth int) {
		if current == nil || depth > 8 || len(parts) >= 16 {
			return
		}
		switch wrapped := current.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				visit(child, depth+1)
			}
		case interface{ Unwrap() error }:
			visit(wrapped.Unwrap(), depth+1)
		}
		text := current.Error()
		if !seen[text] {
			seen[text] = true
			parts = append(parts, text)
		}
	}
	visit(err, 0)
	return strings.Join(parts, "; ")
}
