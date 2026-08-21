package study

import "fmt"

const maxDurableDiagnosticBytes = 4096

func compactRunStateDiagnostics(state *RunState) {
	for i := range state.Tasks {
		task := &state.Tasks[i]
		if task.LastError != nil {
			task.LastError.Message = compactDiagnostic(task.LastError.Message)
			task.LastError.Detail = compactDiagnostic(task.LastError.Detail)
		}
		if task.Validation != nil {
			task.Validation.Message = compactDiagnostic(task.Validation.Message)
		}
		compactAgentMetadata(&task.Agent)
	}
}

func compactAgentMetadata(meta *AgentMetadata) {
	meta.Policy.ExhaustedReason = compactDiagnostic(meta.Policy.ExhaustedReason)
	for i := range meta.Policy.Decisions {
		meta.Policy.Decisions[i].Reason = compactDiagnostic(meta.Policy.Decisions[i].Reason)
		meta.Policy.Decisions[i].Detail = compactDiagnostic(meta.Policy.Decisions[i].Detail)
	}
	for i := range meta.Attempts {
		meta.Attempts[i].ErrorDetail = compactDiagnostic(meta.Attempts[i].ErrorDetail)
	}
	for i := range meta.Permissions.UnsupportedReasons {
		meta.Permissions.UnsupportedReasons[i] = compactDiagnostic(meta.Permissions.UnsupportedReasons[i])
	}
	meta.Cleanup.Error = compactDiagnostic(meta.Cleanup.Error)
	meta.Repair.ExhaustedReason = compactDiagnostic(meta.Repair.ExhaustedReason)
	for i := range meta.Artifacts {
		meta.Artifacts[i].Description = compactDiagnostic(meta.Artifacts[i].Description)
		for key, value := range meta.Artifacts[i].Metadata {
			meta.Artifacts[i].Metadata[key] = compactDiagnostic(value)
		}
	}
	for i := range meta.Warnings {
		meta.Warnings[i] = compactDiagnostic(meta.Warnings[i])
	}
	for i := range meta.Omissions {
		meta.Omissions[i].Field = compactDiagnostic(meta.Omissions[i].Field)
		meta.Omissions[i].Reason = compactDiagnostic(meta.Omissions[i].Reason)
	}
}

func cloneAgentMetadata(meta AgentMetadata) AgentMetadata {
	out := meta
	out.Attempts = append([]AttemptMetadata(nil), meta.Attempts...)
	out.Policy.Decisions = append([]PolicyDecisionMetadata(nil), meta.Policy.Decisions...)
	out.Permissions.UnsupportedReasons = append([]string(nil), meta.Permissions.UnsupportedReasons...)
	out.Artifacts = append([]ArtifactMetadata(nil), meta.Artifacts...)
	for i := range out.Artifacts {
		out.Artifacts[i].Metadata = cloneMetadata(meta.Artifacts[i].Metadata)
	}
	out.Warnings = append([]string(nil), meta.Warnings...)
	out.Omissions = append([]MetadataOmission(nil), meta.Omissions...)
	if meta.Events != nil {
		events := *meta.Events
		out.Events = &events
	}
	if meta.Memory != nil {
		memory := *meta.Memory
		out.Memory = &memory
	}
	if meta.Cost != nil {
		cost := *meta.Cost
		out.Cost = &cost
	}
	if meta.StartedAt != nil {
		started := *meta.StartedAt
		out.StartedAt = &started
	}
	if meta.FinishedAt != nil {
		finished := *meta.FinishedAt
		out.FinishedAt = &finished
	}
	return out
}

func compactDiagnostic(value string) string {
	if len(value) <= maxDurableDiagnosticBytes {
		return value
	}
	return value[:maxDurableDiagnosticBytes] + fmt.Sprintf("... [truncated %d bytes]", len(value)-maxDurableDiagnosticBytes)
}
