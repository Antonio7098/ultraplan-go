package study

import (
	"context"
	"fmt"
)

func (s Service) Synthesize(ctx context.Context, req SynthesisRequest) (ExecutionResult, error) {
	listing, err := s.ListStudy(req.StudyRef)
	if err != nil {
		return ExecutionResult{}, err
	}
	dimension, err := ResolveDimension(listing.Dimensions, req.DimensionRef)
	if err != nil {
		return ExecutionResult{}, err
	}
	result := ExecutionResult{
		Status:     ExecutionStatusCompleted,
		TaskKind:   TaskKindSynthesis,
		Study:      listing.Study,
		Dimension:  dimension,
		OutputPath: FinalReportPath(listing.Study, dimension),
	}
	selectedSources := listing.Sources
	if len(req.SourceRefs) > 0 {
		selectedSources, err = resolveSources(listing.Sources, req.SourceRefs)
		if err != nil {
			return ExecutionResult{}, err
		}
	}
	applicable := GetApplicableSources(selectedSources, dimension)
	for _, source := range applicable {
		validation := ValidateSourceReport(listing.Study, source, dimension)
		result.PreflightResults = append(result.PreflightResults, validation)
		if validation.Status != ValidationStatusPassed {
			result.Blockers = append(result.Blockers, validation.Path)
		}
	}
	if len(result.Blockers) > 0 {
		result.Status = ExecutionStatusPreflightBlocked
		return result, nil
	}
	prompt, err := BuildSynthesisPrompt(PromptRequest{WorkspaceRoot: s.workspaceRoot, Study: listing.Study, Dimension: dimension, Sources: selectedSources})
	if err != nil {
		return ExecutionResult{}, err
	}
	beforeFiles, snapshotErr := snapshotFiles(listing.Study.Path)
	modelOverride := resolveStudyModelOverride(req.Model, listing.Config.Model)
	runtimeResult, runErr := s.startRuntime(ctx, prompt, TaskKindSynthesis, listing.Study, dimension, Source{}, listing.Study.Path, result.OutputPath, beforeFiles, snapshotErr, modelOverride, req.ResumeSession, req.OnSession, req.OnEvent)
	if snapshotErr != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("edit monitoring skipped before runtime: %v", snapshotErr))
	} else if afterFiles, err := snapshotFilesSettled(listing.Study.Path); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("edit monitoring skipped after runtime: %v", err))
	} else {
		result.Warnings = append(result.Warnings, unexpectedEditWarnings(listing.Study.Path, beforeFiles, afterFiles, []string{result.OutputPath})...)
	}
	result.RuntimeRunID = runtimeResult.RunID
	result.RuntimeStatus = runtimeResult.Status
	result.Agent = agentMetadata(runtimeResult, s.runtimeConfig)
	if runErr != nil {
		result.RuntimeError = runErr.Error()
		result.RuntimeErr = runErr
		if runtimeResult.Error != nil {
			result.RuntimeCategory = runtimeResult.Error.Category
			result.RuntimeDetail = runtimeResult.Error.DebugDetail
			if result.RuntimeDetail == "" {
				result.RuntimeDetail = runtimeResult.Error.UserDetail
			}
		}
		result.Validation = ValidateFinalReport(listing.Study, dimension)
		if result.Validation.Status == ValidationStatusPassed {
			result.Status = ExecutionStatusCompleted
			if err := s.deleteCompletedSessions(ctx, runtimeResult); err != nil {
				result.Warnings = append(result.Warnings, err.Error())
			}
			return result, nil
		}
		if recoverableRuntimeOutputFailure(runtimeResult) {
			result.Status = ExecutionStatusValidationFailed
			return result, nil
		}
		result.Status = statusForRuntimeFailure(runtimeResult)
		return result, nil
	}
	result.Validation = ValidateFinalReport(listing.Study, dimension)
	if result.Validation.Status != ValidationStatusPassed {
		result.Status = ExecutionStatusValidationFailed
	} else if err := s.deleteCompletedSessions(ctx, runtimeResult); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	return result, nil
}
