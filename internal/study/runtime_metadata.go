package study

import (
	"fmt"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func agentMetadata(result runtimepkg.Result, req runtimepkg.Request) AgentMetadata {
	meta := AgentMetadata{
		Runtime:  req.Provider,
		RunID:    result.RunID,
		Status:   result.Status,
		Provider: req.Provider,
		Model:    req.Model,
		Policy: PolicyMetadata{
			FinalAttempt:     result.Policy.FinalAttempt,
			FinalTargetIndex: result.Policy.FinalTargetIndex,
			Exhausted:        result.Policy.Exhausted,
			ExhaustedReason:  result.Policy.ExhaustedReason,
		},
		Permissions: PermissionMetadata{
			Mode:               result.Permissions.Mode,
			PolicyID:           result.Permissions.PolicyID,
			Default:            result.Permissions.Default,
			UnsupportedCount:   result.Permissions.UnsupportedCount,
			AuditCount:         result.Permissions.AuditCount,
			UnsupportedReasons: append([]string(nil), result.Permissions.UnsupportedReasons...),
		},
		Cleanup: CleanupMetadata{
			Attempted: result.Cleanup.Attempted,
			Completed: result.Cleanup.Completed,
			Failed:    result.Cleanup.Failed,
		},
		Repair: RepairMetadata{
			Configured:             result.Repair.Configured,
			Attempted:              result.Repair.Attempted,
			MaxAttempts:            result.Repair.MaxAttempts,
			AttemptCount:           result.Repair.AttemptCount,
			Exhausted:              result.Repair.Exhausted,
			ExhaustedReason:        result.Repair.ExhaustedReason,
			PermissionDenied:       result.Repair.PermissionDenied,
			UnsupportedSameSession: result.Repair.UnsupportedSameSession,
		},
		Usage: UsageMetadata{
			InputTokensKnown:  result.Usage.InputTokensKnown,
			InputTokens:       result.Usage.InputTokens,
			OutputTokensKnown: result.Usage.OutputTokensKnown,
			OutputTokens:      result.Usage.OutputTokens,
			TotalTokensKnown:  result.Usage.TotalTokensKnown,
			TotalTokens:       result.Usage.TotalTokens,
			NativeOmitted:     len(result.Usage.Native) > 0,
		},
		Warnings: append([]string(nil), result.Warnings...),
	}
	if result.EventStats.Total > 0 || result.EventStats.Retained > 0 || result.EventStats.Dropped > 0 || result.EventStats.Limit > 0 {
		meta.Events = &EventMetadata{
			Total:    result.EventStats.Total,
			Retained: result.EventStats.Retained,
			Dropped:  result.EventStats.Dropped,
			Limit:    result.EventStats.Limit,
		}
	}
	if result.Memory.Samples > 0 || result.Memory.StartAllocBytes > 0 || result.Memory.PeakAllocBytes > 0 || result.Memory.EndAllocBytes > 0 {
		meta.Memory = &MemoryMetadata{
			StartAllocBytes: result.Memory.StartAllocBytes,
			PeakAllocBytes:  result.Memory.PeakAllocBytes,
			EndAllocBytes:   result.Memory.EndAllocBytes,
			Samples:         result.Memory.Samples,
		}
	}
	if result.Cleanup.Error != nil {
		meta.Cleanup.Error = result.Cleanup.Error.UserDetail
	}
	if result.EstimatedCost != nil {
		meta.Cost = &CostMetadata{Amount: result.EstimatedCost.Amount, Currency: result.EstimatedCost.Currency, Estimate: result.EstimatedCost.Estimate}
	}
	for _, attempt := range result.Attempts {
		item := AttemptMetadata{
			Attempt:         attempt.Attempt,
			AttemptOnTarget: attempt.AttemptOnTarget,
			TargetIndex:     attempt.TargetIndex,
			RunID:           attempt.RunID,
			Status:          attempt.Status,
			Provider:        attempt.Provider,
			Model:           attempt.Model,
			ErrorCategory:   attempt.ErrorCategory,
			RateLimited:     attempt.RateLimited,
		}
		if attempt.RetryAfter > 0 {
			item.RetryAfter = attempt.RetryAfter.String()
		}
		meta.Attempts = append(meta.Attempts, item)
	}
	for _, decision := range result.Policy.Decisions {
		item := PolicyDecisionMetadata{
			Attempt:     decision.Attempt,
			TargetIndex: decision.TargetIndex,
			Kind:        decision.Kind,
			Reason:      decision.Reason,
			Detail:      decision.Detail,
		}
		if decision.Delay > 0 {
			item.Delay = decision.Delay.String()
		}
		meta.Policy.Decisions = append(meta.Policy.Decisions, item)
	}
	for _, artifact := range result.Artifacts {
		meta.Artifacts = append(meta.Artifacts, ArtifactMetadata{
			ID:          artifact.ID,
			URI:         artifact.URI,
			Kind:        artifact.Kind,
			Description: artifact.Description,
			Metadata:    cloneMetadata(artifact.Metadata),
		})
	}
	if len(result.Usage.Native) > 0 {
		meta.Omissions = append(meta.Omissions, MetadataOmission{Field: "usage.native", Reason: "native usage payload omitted from study state"})
	}
	if result.EventStats.Dropped > 0 {
		meta.Omissions = append(meta.Omissions, MetadataOmission{Field: "events", Reason: fmt.Sprintf("%d runtime events omitted from study state after retaining last %d", result.EventStats.Dropped, result.EventStats.Retained)})
	}
	for _, event := range result.Events {
		if event.RawOmitted {
			field := "events.raw"
			if event.ID != "" {
				field = fmt.Sprintf("events.%s.raw", event.ID)
			}
			reason := event.RawOmissionReason
			if reason == "" {
				reason = "runtime raw payload bytes omitted from study state"
			}
			meta.Omissions = append(meta.Omissions, MetadataOmission{Field: field, Reason: reason})
		}
	}
	return meta
}

func cloneMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func retryAfterFromAgent(meta AgentMetadata) *time.Time {
	var out *time.Time
	now := time.Now().UTC()
	for _, decision := range meta.Policy.Decisions {
		if decision.Kind != "retry" || decision.Delay == "" {
			continue
		}
		delay, err := time.ParseDuration(decision.Delay)
		if err != nil {
			continue
		}
		next := now.Add(delay)
		if out == nil || next.Before(*out) {
			out = &next
		}
	}
	for _, attempt := range meta.Attempts {
		if attempt.RetryAfter == "" {
			continue
		}
		delay, err := time.ParseDuration(attempt.RetryAfter)
		if err != nil {
			continue
		}
		next := now.Add(delay)
		if out == nil || next.Before(*out) {
			out = &next
		}
	}
	return out
}
