package sprint

import (
	"context"
	"fmt"
	"strings"

	"github.com/Antonio7098/agentwrap"
	"github.com/Antonio7098/ultraplan-go/internal/project"
)

const generatedArtifactRepairAttempts = 1

func buildCodeContextRepairPrompt(path string, findings []ValidationFinding) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Correct the previous response for UltraPlan artifact `%s`.\n", path)
	fmt.Fprintln(&b, "Return only one complete Markdown document beginning with `# Sprint Code Context`. Do not include a preamble or closing commentary, perform more tool calls, search for the workspace output path, or write any file. UltraPlan owns candidate persistence and promotion.")
	fmt.Fprintf(&b, "Keep the complete response at or below %d bytes. Store source references only; every selected entry needs Path, Lines, and Rationale, with no copied source or fenced code blocks.\n", maxCodeContextBytes)
	fmt.Fprintln(&b, "\nValidation findings:")
	if len(findings) == 0 {
		fmt.Fprintln(&b, "- The response did not satisfy the code-context contract; re-read the required template and return the complete corrected document.")
	} else {
		for _, finding := range findings {
			fmt.Fprintf(&b, "- %s\n", formatValidationFindings([]ValidationFinding{finding}))
		}
	}
	return b.String()
}

func (s Service) requirementsValidationSpec(sp Sprint) *agentwrap.ValidationSpec {
	return generatedArtifactValidationSpec("requirements", ArtifactRelPath(sp, StageRequirements), func() []ValidationFinding {
		data, err := s.store.ReadArtifact(sp, StageRequirements)
		if err != nil {
			return []ValidationFinding{finding("requirements.md", "", ArtifactRelPath(sp, StageRequirements), "missing requirements", err.Error(), "Generate requirements.md.")}
		}
		return ValidateRequirementsContent(data)
	})
}

func (s Service) sprintIndexValidationSpec(sp Sprint, catalog project.ProjectIndex) *agentwrap.ValidationSpec {
	return generatedArtifactValidationSpec("sprint-index", ArtifactRelPath(sp, StageSprintIndex), func() []ValidationFinding {
		inputs, err := s.store.ReadPlanningInputs(sp)
		if err != nil {
			return []ValidationFinding{finding("sprint-index.md", "", ArtifactRelPath(sp, StageSprintIndex), "missing sprint index", err.Error(), "Generate sprint-index.md.")}
		}
		_, findings := ValidateSprintIndexContent(inputs.SprintIndex, catalog)
		return findings
	})
}

func (s Service) technicalHandbookValidationSpec(sp Sprint, manifest HandbookManifest) *agentwrap.ValidationSpec {
	return generatedArtifactValidationSpec("technical-handbook", ArtifactRelPath(sp, StageTechnicalHandbook), func() []ValidationFinding {
		data, err := s.store.ReadArtifact(sp, StageTechnicalHandbook)
		if err != nil {
			return []ValidationFinding{finding("technical-handbook.md", "", ArtifactRelPath(sp, StageTechnicalHandbook), "missing technical handbook", err.Error(), "Generate technical-handbook.md.")}
		}
		return ValidateTechnicalHandbookContent(data, manifest)
	})
}

func (s Service) areaReasoningEntryValidationSpec(manifest ReasoningManifest, entry ReasoningTemplateEntry) *agentwrap.ValidationSpec {
	return generatedArtifactValidationSpec("area-reasoning-"+slugReviewID(entry.Name), entry.OutputPath, func() []ValidationFinding {
		return s.areaReasoningEntryFindings(manifest, entry)
	})
}

func (s Service) finalReasoningValidationSpec(sp Sprint, manifest ReasoningManifest) *agentwrap.ValidationSpec {
	return generatedArtifactValidationSpec("reasoning", ArtifactRelPath(sp, StageReasoning), func() []ValidationFinding {
		data, err := s.store.ReadArtifact(sp, StageReasoning)
		if err != nil {
			return []ValidationFinding{finding("reasoning.md", "", ArtifactRelPath(sp, StageReasoning), "missing final reasoning", err.Error(), "Generate reasoning.md.")}
		}
		return ValidateFinalReasoningContent(data, manifest)
	})
}

func (s Service) planValidationSpec(sp Sprint, manifest PlanManifest) *agentwrap.ValidationSpec {
	return generatedArtifactValidationSpec("plan", ArtifactRelPath(sp, StagePlan), func() []ValidationFinding {
		data, err := s.store.ReadArtifact(sp, StagePlan)
		if err != nil {
			return []ValidationFinding{finding("plan.md", "", ArtifactRelPath(sp, StagePlan), "missing plan", err.Error(), "Generate plan.md.")}
		}
		return ValidatePlanContent(data, manifest)
	})
}

func generatedArtifactValidationSpec(id, path string, validate func() []ValidationFinding) *agentwrap.ValidationSpec {
	return &agentwrap.ValidationSpec{
		Validators: []agentwrap.Validator{agentwrap.ValidatorFunc(func(ctx context.Context, _ agentwrap.ValidationContext) agentwrap.ValidationCheck {
			select {
			case <-ctx.Done():
				return agentwrap.ValidationCheck{
					ExpectationID: id,
					Kind:          agentwrap.ExpectationCustom,
					Severity:      agentwrap.ExpectationRequired,
					Passed:        false,
					Expected:      "valid generated artifact",
					Observed:      ctx.Err().Error(),
					Detail:        "validation cancelled",
					RepairHint:    "Retry generation when the context is active.",
				}
			default:
			}
			findings := validate()
			if len(findings) == 0 {
				return agentwrap.ValidationCheck{
					ExpectationID: id,
					Kind:          agentwrap.ExpectationCustom,
					Severity:      agentwrap.ExpectationRequired,
					Passed:        true,
					Expected:      "valid generated artifact",
					Observed:      "valid",
					Detail:        path + " passed UltraPlan validation",
				}
			}
			return agentwrap.ValidationCheck{
				ExpectationID: id,
				Kind:          agentwrap.ExpectationCustom,
				Severity:      agentwrap.ExpectationRequired,
				Passed:        false,
				Expected:      "valid generated artifact",
				Observed:      formatValidationFindings(findings),
				Detail:        path + " failed UltraPlan validation",
				RepairHint:    "Update only " + path + " so it satisfies the listed UltraPlan validation findings.",
			}
		})},
		Repair: agentwrap.RepairConfig{
			MaxAttempts:                 generatedArtifactRepairAttempts + 1,
			SessionAction:               agentwrap.SessionActionContinue,
			AllowFreshSessionFallback:   true,
			FreshSessionFallbackOnError: true,
			BuildPrompt: func(ctx agentwrap.RepairContext) string {
				return buildGeneratedArtifactRepairPrompt(path, ctx.Validation.Failures)
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

func buildGeneratedArtifactRepairPrompt(path string, failures []agentwrap.ValidationFailure) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Repair the generated UltraPlan artifact `%s` so validation passes.\n\n", path)
	fmt.Fprintln(&b, "Use the same sprint context and previous work. Update only the artifact named above unless a listed validation finding explicitly requires another selected output.")
	fmt.Fprintln(&b, "\nValidation findings:")
	if len(failures) == 0 {
		fmt.Fprintln(&b, "- validation failed, but no detailed findings were provided; inspect the artifact and satisfy the required template sections.")
	} else {
		for _, failure := range failures {
			fmt.Fprintf(&b, "- %s: %s\n", failure.ExpectationID, strings.TrimSpace(failure.Observed))
			if strings.TrimSpace(failure.RepairHint) != "" {
				fmt.Fprintf(&b, "  Fix: %s\n", strings.TrimSpace(failure.RepairHint))
			}
		}
	}
	fmt.Fprintln(&b, "\nAfter editing, stop. Do not modify source repositories, Git state, workspace config, or unrelated sprint artifacts.")
	return b.String()
}

func buildGeneratedArtifactRepairPromptFromFindings(path string, findings []ValidationFinding) string {
	failures := make([]agentwrap.ValidationFailure, 0, len(findings))
	for _, finding := range findings {
		failures = append(failures, agentwrap.ValidationFailure{
			ExpectationID: finding.Section,
			Kind:          agentwrap.ExpectationCustom,
			Severity:      agentwrap.ExpectationRequired,
			Expected:      "valid generated artifact",
			Observed:      formatValidationFindings([]ValidationFinding{finding}),
			Detail:        finding.Problem,
			RepairHint:    finding.Suggestion,
		})
	}
	return buildGeneratedArtifactRepairPrompt(path, failures)
}

func formatValidationFindings(findings []ValidationFinding) string {
	var b strings.Builder
	for i, finding := range findings {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s", finding.Section)
		if finding.EntryName != "" {
			fmt.Fprintf(&b, " %q", finding.EntryName)
		}
		if finding.Path != "" {
			fmt.Fprintf(&b, " (%s)", finding.Path)
		}
		if finding.Problem != "" {
			fmt.Fprintf(&b, ": %s", finding.Problem)
		}
		if finding.Cause != "" {
			fmt.Fprintf(&b, "; %s", finding.Cause)
		}
		if finding.Suggestion != "" {
			fmt.Fprintf(&b, "; fix: %s", finding.Suggestion)
		}
	}
	return b.String()
}
