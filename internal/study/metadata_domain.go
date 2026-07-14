package study

import "time"

type AgentMetadata struct {
	Runtime     string             `json:"runtime,omitempty"`
	RunID       string             `json:"run_id,omitempty"`
	Status      string             `json:"status,omitempty"`
	Provider    string             `json:"provider,omitempty"`
	Model       string             `json:"model,omitempty"`
	Attempts    []AttemptMetadata  `json:"attempts,omitempty"`
	Policy      PolicyMetadata     `json:"policy,omitempty"`
	Permissions PermissionMetadata `json:"permissions,omitempty"`
	Cleanup     CleanupMetadata    `json:"cleanup,omitempty"`
	Repair      RepairMetadata     `json:"repair,omitempty"`
	Usage       UsageMetadata      `json:"usage,omitempty"`
	Events      *EventMetadata     `json:"events,omitempty"`
	Memory      *MemoryMetadata    `json:"memory,omitempty"`
	Cost        *CostMetadata      `json:"cost,omitempty"`
	Artifacts   []ArtifactMetadata `json:"artifacts,omitempty"`
	Warnings    []string           `json:"warnings,omitempty"`
	Omissions   []MetadataOmission `json:"omissions,omitempty"`
	StartedAt   *time.Time         `json:"started_at,omitempty"`
	FinishedAt  *time.Time         `json:"finished_at,omitempty"`
	DurationMS  int64              `json:"duration_ms,omitempty"`
}

type EventMetadata struct {
	Total    int64 `json:"total,omitempty"`
	Retained int   `json:"retained,omitempty"`
	Dropped  int64 `json:"dropped,omitempty"`
	Limit    int   `json:"limit,omitempty"`
}

type MemoryMetadata struct {
	StartAllocBytes uint64 `json:"start_alloc_bytes,omitempty"`
	PeakAllocBytes  uint64 `json:"peak_alloc_bytes,omitempty"`
	EndAllocBytes   uint64 `json:"end_alloc_bytes,omitempty"`
	Samples         int64  `json:"samples,omitempty"`
}

type AttemptMetadata struct {
	Attempt         int    `json:"attempt"`
	AttemptOnTarget int    `json:"attempt_on_target,omitempty"`
	TargetIndex     int    `json:"target_index,omitempty"`
	RunID           string `json:"run_id,omitempty"`
	Status          string `json:"status,omitempty"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ErrorCategory   string `json:"error_category,omitempty"`
	RateLimited     bool   `json:"rate_limited,omitempty"`
	RetryAfter      string `json:"retry_after,omitempty"`
}

type PolicyMetadata struct {
	FinalAttempt     int                      `json:"final_attempt,omitempty"`
	FinalTargetIndex int                      `json:"final_target_index,omitempty"`
	Exhausted        bool                     `json:"exhausted,omitempty"`
	ExhaustedReason  string                   `json:"exhausted_reason,omitempty"`
	Decisions        []PolicyDecisionMetadata `json:"decisions,omitempty"`
}

type PolicyDecisionMetadata struct {
	Attempt     int    `json:"attempt,omitempty"`
	TargetIndex int    `json:"target_index,omitempty"`
	Kind        string `json:"kind,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Detail      string `json:"detail,omitempty"`
	Delay       string `json:"delay,omitempty"`
}

type PermissionMetadata struct {
	Mode               string   `json:"mode,omitempty"`
	PolicyID           string   `json:"policy_id,omitempty"`
	Default            string   `json:"default,omitempty"`
	UnsupportedCount   int      `json:"unsupported_count,omitempty"`
	AuditCount         int      `json:"audit_count,omitempty"`
	UnsupportedReasons []string `json:"unsupported_reasons,omitempty"`
}

type CleanupMetadata struct {
	Attempted bool   `json:"attempted,omitempty"`
	Completed bool   `json:"completed,omitempty"`
	Failed    bool   `json:"failed,omitempty"`
	Error     string `json:"error,omitempty"`
}

type RepairMetadata struct {
	Configured             bool   `json:"configured,omitempty"`
	Attempted              bool   `json:"attempted,omitempty"`
	MaxAttempts            int    `json:"max_attempts,omitempty"`
	AttemptCount           int    `json:"attempt_count,omitempty"`
	Exhausted              bool   `json:"exhausted,omitempty"`
	ExhaustedReason        string `json:"exhausted_reason,omitempty"`
	PermissionDenied       bool   `json:"permission_denied,omitempty"`
	UnsupportedSameSession bool   `json:"unsupported_same_session,omitempty"`
}

type UsageMetadata struct {
	InputTokensKnown      bool  `json:"input_tokens_known"`
	InputTokens           int64 `json:"input_tokens,omitempty"`
	OutputTokensKnown     bool  `json:"output_tokens_known"`
	OutputTokens          int64 `json:"output_tokens,omitempty"`
	TotalTokensKnown      bool  `json:"total_tokens_known"`
	TotalTokens           int64 `json:"total_tokens,omitempty"`
	ReasoningTokensKnown  bool  `json:"reasoning_tokens_known"`
	ReasoningTokens       int64 `json:"reasoning_tokens,omitempty"`
	CacheReadTokensKnown  bool  `json:"cache_read_tokens_known"`
	CacheReadTokens       int64 `json:"cache_read_tokens,omitempty"`
	CacheWriteTokensKnown bool  `json:"cache_write_tokens_known"`
	CacheWriteTokens      int64 `json:"cache_write_tokens,omitempty"`
	TurnsKnown            bool  `json:"turns_known"`
	Turns                 int64 `json:"turns,omitempty"`
	NativeOmitted         bool  `json:"native_omitted,omitempty"`
}

type CostMetadata struct {
	Amount   float64 `json:"amount"`
	Currency string  `json:"currency,omitempty"`
	Estimate bool    `json:"estimate,omitempty"`
}

type ArtifactMetadata struct {
	ID          string            `json:"id,omitempty"`
	URI         string            `json:"uri,omitempty"`
	Kind        string            `json:"kind,omitempty"`
	Description string            `json:"description,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type MetadataOmission struct {
	Field  string `json:"field"`
	Reason string `json:"reason"`
}
