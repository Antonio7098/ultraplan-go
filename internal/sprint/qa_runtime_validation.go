package sprint

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Antonio7098/agentwrap"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type qaOutputCapture struct {
	mu      sync.RWMutex
	content string
}

func (c *qaOutputCapture) observe(payload map[string]any) {
	content := qaCapturedOutput(payload, 0)
	if content == "" {
		return
	}
	c.mu.Lock()
	c.content = content
	c.mu.Unlock()
}

func (c *qaOutputCapture) load() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.content
}

func qaCapturedOutput(value any, depth int) string {
	if depth > 5 {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"structured_output", "output", "content", "text", "message", "part"} {
			if nested, ok := typed[key]; ok {
				if content := qaCapturedOutput(nested, depth+1); content != "" {
					return content
				}
			}
		}
	case []any:
		for i := len(typed) - 1; i >= 0; i-- {
			if content := qaCapturedOutput(typed[i], depth+1); content != "" {
				return content
			}
		}
	case string:
		return typed
	}
	return ""
}

func qaInvestigatorValidationSpec(budgets QABudgets, capture *qaOutputCapture) *agentwrap.ValidationSpec {
	return &agentwrap.ValidationSpec{
		Validators: []agentwrap.Validator{agentwrap.ValidatorFunc(func(ctx context.Context, vctx agentwrap.ValidationContext) agentwrap.ValidationCheck {
			check := agentwrap.ValidationCheck{
				ExpectationID: "qa-investigator-output",
				Kind:          agentwrap.ExpectationCustom,
				Severity:      agentwrap.ExpectationRequired,
				Expected:      "one strict schema_version 1 QA investigator JSON object",
				RepairHint:    "Return only the corrected canonical JSON object and do not perform more tool calls.",
			}
			if err := ctx.Err(); err != nil {
				check.Observed, check.Detail = err.Error(), "QA investigator validation cancelled"
				return check
			}
			content := vctx.Result.TerminalOutput
			if content == "" && capture != nil {
				content = capture.load()
			}
			_, diagnostic, err := decodeQAInvestigatorOutput(pruntime.Result{Status: string(vctx.Result.Status), SessionID: string(vctx.Result.SessionID), TerminalOutput: content}, budgets.ShardOutputBytes)
			if err == nil {
				check.Passed, check.Observed, check.Detail = true, "valid", "QA investigator output passed strict decoding"
				return check
			}
			check.Observed = qaValidationDiagnostic(diagnostic, err)
			check.Detail = "QA investigator output failed strict decoding"
			return check
		})},
		Repair: agentwrap.RepairConfig{
			MaxAttempts:                 budgets.OutputRepairAttempts,
			SessionAction:               agentwrap.SessionActionContinue,
			AllowFreshSessionFallback:   true,
			FreshSessionFallbackOnError: true,
			BuildPrompt: func(ctx agentwrap.RepairContext) string {
				return qaValidationRepairPrompt(ctx.Validation.Failures)
			},
			OverrideRequest: func(ctx agentwrap.RepairContext, req agentwrap.RunRequest) agentwrap.RunRequest {
				if ctx.Attempt >= 2 {
					req.SessionID = ""
					req.SessionAction = agentwrap.SessionActionFresh
				}
				return req
			},
		},
	}
}

func qaValidationDiagnostic(diagnostic QAOutputDiagnostic, err error) string {
	parts := []string{"kind=" + diagnostic.Kind, fmt.Sprintf("output_bytes=%d", diagnostic.OutputBytes)}
	if diagnostic.Detail != "" {
		parts = append(parts, diagnostic.Detail)
	} else if err != nil {
		parts = append(parts, qaSafeDiagnostic(err.Error()))
	}
	return strings.Join(parts, "; ")
}

func qaValidationRepairPrompt(failures []agentwrap.ValidationFailure) string {
	var details []string
	for _, failure := range failures {
		if observed := strings.TrimSpace(failure.Observed); observed != "" {
			details = append(details, qaSafeDiagnostic(observed))
		}
	}
	if len(details) == 0 {
		details = append(details, "the previous output failed strict QA validation")
	}
	return "Return only one corrected strict QA JSON object. Do not perform more tool calls. Unknown fields and text outside the object are rejected. Required top-level fields are schema_version, theories, evidence, context_requests, and check_requests. schema_version must be 1 and the other fields must be arrays. A valid minimal object is {\"schema_version\":1,\"theories\":[],\"evidence\":[],\"context_requests\":[],\"check_requests\":[]}. Rejection: " + strings.Join(details, "; ")
}
