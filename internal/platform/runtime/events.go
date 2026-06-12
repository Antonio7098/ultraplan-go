package runtime

import "github.com/Antonio7098/agentwrap"

func rawOmissionReason(present, safe bool) string {
	if !present {
		return ""
	}
	if safe {
		return "raw payload bytes omitted by UltraPlan runtime mapping"
	}
	return "unsafe raw payload bytes omitted by default"
}

func rawSource(raw *agentwrap.RawPayload) string {
	if raw == nil {
		return ""
	}
	return raw.Source
}

func rawEncoding(raw *agentwrap.RawPayload) string {
	if raw == nil {
		return ""
	}
	return raw.Encoding
}

func mapArtifacts(values []agentwrap.ArtifactRef) []Artifact {
	out := make([]Artifact, 0, len(values))
	for _, value := range values {
		out = append(out, Artifact{
			ID:          string(value.ID),
			URI:         value.URI,
			Kind:        value.Kind,
			Description: value.Description,
			Metadata:    cloneStringMap(value.Metadata),
		})
	}
	return out
}

func mapCost(value *agentwrap.CostEstimate) *CostEstimate {
	if value == nil {
		return nil
	}
	return &CostEstimate{Amount: value.Amount, Currency: value.Currency, Estimate: value.Estimate}
}

func mapAttempts(values []agentwrap.AttemptSummary) []AttemptSummary {
	out := make([]AttemptSummary, 0, len(values))
	for _, value := range values {
		item := AttemptSummary{
			Attempt:         value.Attempt,
			AttemptOnTarget: value.AttemptOnTarget,
			TargetIndex:     value.TargetIndex,
			RunID:           string(value.RunID),
			Status:          string(value.Status),
			Provider:        string(value.Context.Provider),
			Model:           string(value.Context.Model),
			ErrorCategory:   string(value.ErrorCategory),
		}
		if value.RateLimit != nil {
			item.RateLimited = true
			item.RetryAfter = value.RateLimit.RetryAfter
		}
		out = append(out, item)
	}
	return out
}

func mapPolicy(value agentwrap.PolicyMetadata) PolicySummary {
	out := PolicySummary{
		FinalAttempt:     value.FinalAttempt,
		FinalTargetIndex: value.FinalTargetIndex,
		Exhausted:        value.Exhausted,
		ExhaustedReason:  value.ExhaustedReason,
	}
	for _, decision := range value.Decisions {
		out.Decisions = append(out.Decisions, PolicyDecision{
			Attempt:     decision.Attempt,
			TargetIndex: decision.TargetIndex,
			Kind:        string(decision.Kind),
			Reason:      decision.Reason,
			Detail:      decision.Detail,
			Delay:       decision.Delay,
		})
	}
	return out
}

func mapValidation(value agentwrap.ValidationMetadata) ValidationSummary {
	return ValidationSummary{
		Configured: value.Configured,
		Passed:     value.Final.Passed,
		Failures:   value.Final.FailedCount,
		Errors:     len(value.Final.Errors),
	}
}

func mapPermissions(value agentwrap.PermissionMetadata) PermissionSummary {
	out := PermissionSummary{
		Mode:             string(value.Mode),
		PolicyID:         value.PolicyID,
		Default:          string(value.Policy.Default),
		UnsupportedCount: len(value.Unsupported),
		AuditCount:       len(value.Audit),
	}
	for _, unsupported := range value.Unsupported {
		if unsupported.Reason != "" {
			out.UnsupportedReasons = append(out.UnsupportedReasons, unsupported.Reason)
		}
	}
	return out
}

func mapCleanup(value agentwrap.CleanupMetadata) CleanupSummary {
	return CleanupSummary{
		Attempted: value.Attempted,
		Completed: value.Completed,
		Failed:    value.Failed,
		Error:     mapSDKError(value.Error),
	}
}

func mapRepair(value agentwrap.RepairMetadata) RepairSummary {
	return RepairSummary{
		Configured:             value.Configured,
		Attempted:              value.Attempted,
		MaxAttempts:            value.MaxAttempts,
		AttemptCount:           len(value.Attempts),
		Exhausted:              value.Exhausted,
		ExhaustedReason:        value.ExhaustedReason,
		PermissionDenied:       value.PermissionDenied,
		UnsupportedSameSession: value.UnsupportedSameSession,
	}
}
