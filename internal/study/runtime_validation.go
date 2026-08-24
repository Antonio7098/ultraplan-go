package study

import (
	"context"
	"fmt"
	"strings"

	"github.com/Antonio7098/agentwrap"
)

func studyReportValidationSpec(kind TaskKind, study Study, source Source, dimension Dimension, outputPath string) *agentwrap.ValidationSpec {
	validate := func() ValidationResult {
		if kind == TaskKindSynthesis {
			return ValidateFinalReport(study, dimension)
		}
		return ValidateSourceReport(study, source, dimension)
	}
	return &agentwrap.ValidationSpec{
		Expectations: []agentwrap.ValidationExpectation{{
			ID:         "expected_output",
			Kind:       agentwrap.ExpectationFile,
			Severity:   agentwrap.ExpectationRequired,
			Path:       outputPath,
			RepairHint: "Create or repair only the expected report.",
		}},
		Validators: []agentwrap.Validator{agentwrap.ValidatorFunc(func(ctx context.Context, _ agentwrap.ValidationContext) agentwrap.ValidationCheck {
			if err := ctx.Err(); err != nil {
				return agentwrap.ValidationCheck{ExpectationID: "report_schema", Kind: agentwrap.ExpectationCustom, Severity: agentwrap.ExpectationRequired, Expected: "valid report", Observed: err.Error(), Detail: "validation cancelled"}
			}
			result := validate()
			check := agentwrap.ValidationCheck{ExpectationID: "report_schema", Kind: agentwrap.ExpectationCustom, Severity: agentwrap.ExpectationRequired, Expected: "report satisfying the selected study schema"}
			if result.Status == ValidationStatusPassed {
				check.Passed = true
				check.Observed = "valid"
				check.Detail = "report passed UltraPlan study validation"
				return check
			}
			check.Observed = studyValidationFailureText(result)
			check.Detail = "report failed UltraPlan study validation"
			check.RepairHint = "Repair only " + outputPath + " using the listed failed checks."
			return check
		})},
		Repair: agentwrap.RepairConfig{
			MaxAttempts:                 2,
			SessionAction:               agentwrap.SessionActionContinue,
			AllowFreshSessionFallback:   true,
			FreshSessionFallbackOnError: true,
			BuildPrompt: func(ctx agentwrap.RepairContext) string {
				return buildStudyReportRepairPrompt(outputPath, ctx.Validation.Failures)
			},
		},
	}
}

func studyValidationFailureText(result ValidationResult) string {
	var failures []string
	for _, check := range result.Checks {
		if check.Status != ValidationStatusFailed {
			continue
		}
		name := check.Name
		if name == "" {
			name = check.ID
		}
		failures = append(failures, fmt.Sprintf("%s: expected %s; observed %s; guidance %s", name, check.Expected, check.Observed, check.Guidance))
		if len(failures) >= 12 {
			break
		}
	}
	if len(failures) == 0 && result.Err != nil {
		return result.Err.Error()
	}
	return strings.Join(failures, "; ")
}

func buildStudyReportRepairPrompt(outputPath string, failures []agentwrap.ValidationFailure) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repair only the generated study report `%s` so validation passes. Preserve supported evidence and citations.\n", outputPath)
	for _, failure := range failures {
		if observed := strings.TrimSpace(failure.Observed); observed != "" {
			fmt.Fprintf(&b, "- %s\n", observed)
		}
	}
	fmt.Fprintln(&b, "After editing the report, stop. Do not modify sources, workspace configuration, Git state, or unrelated reports.")
	return b.String()
}
