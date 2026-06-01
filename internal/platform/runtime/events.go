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
