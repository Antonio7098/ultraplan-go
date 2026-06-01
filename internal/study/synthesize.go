package study

import (
	"context"
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
		OutputPath: FinalReportPath(listing.Study),
	}
	applicable := GetApplicableSources(listing.Sources, dimension)
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
	prompt, err := BuildSynthesisPrompt(PromptRequest{WorkspaceRoot: s.workspaceRoot, Study: listing.Study, Dimension: dimension})
	if err != nil {
		return ExecutionResult{}, err
	}
	runtimeResult, runErr := s.startRuntime(ctx, prompt, TaskKindSynthesis, listing.Study, dimension, Source{}, listing.Study.Path, result.OutputPath)
	result.RuntimeRunID = runtimeResult.RunID
	result.RuntimeStatus = runtimeResult.Status
	if runErr != nil {
		result.Status = statusForRuntimeFailure(runtimeResult)
		result.RuntimeError = runErr.Error()
		result.RuntimeErr = runErr
		if runtimeResult.Error != nil {
			result.RuntimeCategory = runtimeResult.Error.Category
		}
		return result, nil
	}
	result.Validation = ValidateFinalReport(listing.Study)
	if result.Validation.Status != ValidationStatusPassed {
		result.Status = ExecutionStatusValidationFailed
	}
	return result, nil
}
