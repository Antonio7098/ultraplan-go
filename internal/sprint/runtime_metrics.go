package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
)

const (
	runtimeMetricsSchemaVersion = 2
	runtimeMetricsFileName      = ".runtime-metrics.json"
)

type RuntimeTokenMetric struct {
	Known bool  `json:"known"`
	Value int64 `json:"value,omitempty"`
}

type SprintRuntimeMetric struct {
	Sequence            int                `json:"sequence"`
	StageSequence       int                `json:"stage_sequence"`
	Stage               PlanningStage      `json:"stage"`
	Operation           string             `json:"operation,omitempty"`
	Role                string             `json:"role,omitempty"`
	Task                string             `json:"task,omitempty"`
	Coverage            string             `json:"coverage,omitempty"`
	QAAttemptID         string             `json:"qa_attempt_id,omitempty"`
	OperationalAttempt  string             `json:"operational_attempt_id,omitempty"`
	MapID               string             `json:"map_id,omitempty"`
	ShardID             string             `json:"shard_id,omitempty"`
	ArbiterGroupID      string             `json:"arbiter_group_id,omitempty"`
	EvidenceID          string             `json:"evidence_id,omitempty"`
	IssueID             string             `json:"issue_id,omitempty"`
	RepairRunID         string             `json:"repair_run_id,omitempty"`
	Cycle               string             `json:"cycle,omitempty"`
	Call                string             `json:"call,omitempty"`
	CallID              string             `json:"call_id"`
	TraceID             string             `json:"trace_id,omitempty"`
	RunID               string             `json:"run_id,omitempty"`
	SessionID           string             `json:"session_id,omitempty"`
	TurnID              string             `json:"turn_id,omitempty"`
	SessionAction       string             `json:"session_action,omitempty"`
	Status              string             `json:"status,omitempty"`
	Provider            string             `json:"provider,omitempty"`
	Model               string             `json:"model,omitempty"`
	Variant             string             `json:"variant,omitempty"`
	Fallback            bool               `json:"fallback"`
	FinalTargetIndex    int                `json:"final_target_index"`
	Sandbox             string             `json:"sandbox,omitempty"`
	PromptBytes         int                `json:"prompt_bytes"`
	SharedPrefixBytes   int                `json:"shared_prefix_bytes,omitempty"`
	StageSuffixBytes    int                `json:"stage_suffix_bytes"`
	SharedPrefixDigest  string             `json:"shared_prefix_sha256,omitempty"`
	CacheKey            string             `json:"cache_key,omitempty"`
	CacheMode           string             `json:"cache_mode,omitempty"`
	CacheTransport      string             `json:"cache_transport,omitempty"`
	InputTokens         RuntimeTokenMetric `json:"input_tokens"`
	OutputTokens        RuntimeTokenMetric `json:"output_tokens"`
	ReasoningTokens     RuntimeTokenMetric `json:"reasoning_tokens"`
	CacheReadTokens     RuntimeTokenMetric `json:"cache_read_tokens"`
	CacheWriteTokens    RuntimeTokenMetric `json:"cache_write_tokens"`
	TotalTokens         RuntimeTokenMetric `json:"total_tokens"`
	Turns               RuntimeTokenMetric `json:"turns"`
	CostAmount          float64            `json:"cost_amount,omitempty"`
	CostCurrency        string             `json:"cost_currency,omitempty"`
	CostEstimated       bool               `json:"cost_estimated,omitempty"`
	CostSource          string             `json:"cost_source,omitempty"`
	StartedAt           time.Time          `json:"started_at,omitempty"`
	FinishedAt          time.Time          `json:"finished_at,omitempty"`
	ErrorCategory       string             `json:"error_category,omitempty"`
	ErrorDetail         string             `json:"error_detail,omitempty"`
	ToolCalls           int                `json:"tool_calls"`
	ToolCallsByKind     map[string]int     `json:"tool_calls_by_kind,omitempty"`
	ToolCallCountExact  bool               `json:"tool_call_count_exact"`
	RuntimeEvents       int64              `json:"runtime_events"`
	RetainedEvents      int                `json:"retained_events"`
	DroppedEvents       int64              `json:"dropped_events"`
	RuntimeAttempts     int                `json:"runtime_attempts"`
	WarningCount        int                `json:"warning_count"`
	PermissionMode      string             `json:"permission_mode,omitempty"`
	PermissionDefault   string             `json:"permission_default,omitempty"`
	UnsupportedTools    int                `json:"unsupported_tools"`
	PermissionAudits    int                `json:"permission_audits"`
	DistinctFilesRead   int                `json:"distinct_files_read"`
	ReadBytes           int64              `json:"read_bytes"`
	RepeatedReads       int                `json:"repeated_reads"`
	SearchCalls         int                `json:"search_calls"`
	Continuation        bool               `json:"continuation"`
	ContinuationBytes   int                `json:"continuation_bytes,omitempty"`
	ParentRunID         string             `json:"parent_run_id,omitempty"`
	RepairOf            string             `json:"repair_of,omitempty"`
	RetryOf             string             `json:"retry_of,omitempty"`
	DurationMs          int64              `json:"duration_ms,omitempty"`
	TimeToFirstOutputMs int64              `json:"time_to_first_output_ms,omitempty"`
}

type SprintRuntimeMetrics struct {
	SchemaVersion int                   `json:"schema_version"`
	Project       string                `json:"project"`
	Sprint        string                `json:"sprint"`
	UpdatedAt     time.Time             `json:"updated_at"`
	Runs          []SprintRuntimeMetric `json:"runs"`
}

func RuntimeMetricsRelPath(sp Sprint) string {
	return filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug, runtimeMetricsFileName))
}

func LoadRuntimeMetrics(root string, sp Sprint) (SprintRuntimeMetrics, error) {
	path, err := resolveSprintContained(root, sp, RuntimeMetricsRelPath(sp))
	if err != nil {
		return SprintRuntimeMetrics{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SprintRuntimeMetrics{}, err
	}
	var metrics SprintRuntimeMetrics
	if err := json.Unmarshal(data, &metrics); err != nil {
		return SprintRuntimeMetrics{}, fmt.Errorf("decode sprint runtime metrics: %w", err)
	}
	if (metrics.SchemaVersion != 1 && metrics.SchemaVersion != runtimeMetricsSchemaVersion) || metrics.Project != sp.Project || metrics.Sprint != sp.Slug {
		return SprintRuntimeMetrics{}, fmt.Errorf("invalid sprint runtime metrics identity")
	}
	// Version 2 adds call identity and runtime-event accounting. The token
	// fields in version 1 are preserved exactly when the ledger is upgraded.
	metrics.SchemaVersion = runtimeMetricsSchemaVersion
	stageSequences := map[PlanningStage]int{}
	for index := range metrics.Runs {
		stageSequences[metrics.Runs[index].Stage]++
		if metrics.Runs[index].Sequence == 0 {
			metrics.Runs[index].Sequence = index + 1
		}
		if metrics.Runs[index].StageSequence == 0 {
			metrics.Runs[index].StageSequence = stageSequences[metrics.Runs[index].Stage]
		}
		if metrics.Runs[index].CallID == "" {
			metrics.Runs[index].CallID = fmt.Sprintf("legacy-runtime-call-%d", index+1)
		}
	}
	return metrics, nil
}

func (s Service) RuntimeMetrics(projectRef, sprintRef string) (SprintRuntimeMetrics, error) {
	projects, err := project.DiscoverProjects(s.root)
	if err != nil {
		return SprintRuntimeMetrics{}, err
	}
	p, err := project.ResolveProject(projects, projectRef)
	if err != nil {
		return SprintRuntimeMetrics{}, err
	}
	sprints, err := DiscoverSprints(s.root, p)
	if err != nil {
		return SprintRuntimeMetrics{}, err
	}
	sp, err := ResolveSprint(sprints, sprintRef)
	if err != nil {
		return SprintRuntimeMetrics{}, err
	}
	if !inside(p.Path, sp.Path) {
		return SprintRuntimeMetrics{}, fmt.Errorf("sprint path mismatch for %q", sp.Slug)
	}
	metrics, err := LoadRuntimeMetrics(s.root, sp)
	if errors.Is(err, os.ErrNotExist) {
		return SprintRuntimeMetrics{SchemaVersion: runtimeMetricsSchemaVersion, Project: sp.Project, Sprint: sp.Slug}, nil
	}
	return metrics, err
}

func (s Service) startSprintRuntime(ctx context.Context, sp Sprint, stage PlanningStage, req pruntime.Request) (pruntime.Result, error) {
	// Retry cleanup that was interrupted by a crash before admitting more work.
	// Recent failed stores remain available for session recovery.
	pruntime.CleanupRuntimeStores(sp.Path, 72*time.Hour, 2*1024*1024*1024, false)
	result, runErr := s.runtime.StartRun(ctx, req)
	if metricErr := s.recordRuntimeMetric(sp, stage, req, result); metricErr != nil {
		result.Warnings = append(result.Warnings, "runtime metrics were not persisted: "+safeError(metricErr))
		if stage == PlanningStage(VerificationPhaseQA) || stage == PlanningStage(VerificationPhaseRepair) {
			runErr = errors.Join(runErr, fmt.Errorf("persist required %s runtime metrics: %w", stage, metricErr))
		}
	}
	return result, runErr
}

func (s Service) recordRuntimeMetric(sp Sprint, stage PlanningStage, req pruntime.Request, result pruntime.Result) error {
	if s.metricsMu != nil {
		s.metricsMu.Lock()
		defer s.metricsMu.Unlock()
	}
	metrics, err := LoadRuntimeMetrics(s.root, sp)
	if errors.Is(err, os.ErrNotExist) {
		metrics = SprintRuntimeMetrics{SchemaVersion: runtimeMetricsSchemaVersion, Project: sp.Project, Sprint: sp.Slug}
	} else if err != nil {
		return err
	}
	explanation := explainComposedPrompt(req.Prompt)
	if req.Cache.BreakpointBytes > 0 {
		explanation.SharedPrefixBytes = req.Cache.BreakpointBytes
		explanation.StageSuffixBytes = len(req.Prompt) - req.Cache.BreakpointBytes
		explanation.SharedPrefixDigest = req.Cache.PrefixDigest
		explanation.CacheKey = req.Cache.Key
	}
	toolKinds, firstOutput := runtimeEventMetrics(result)
	contextMetrics := qaAttemptContextMetrics(req, result.Events, 0)
	toolCalls := 0
	for _, count := range toolKinds {
		toolCalls += count
	}
	stageSequence := 1
	for _, prior := range metrics.Runs {
		if prior.Stage == stage {
			stageSequence++
		}
	}
	record := SprintRuntimeMetric{
		Sequence: len(metrics.Runs) + 1, StageSequence: stageSequence,
		Stage: stage, Operation: req.Metadata["operation"], Role: req.Metadata["role"], Task: req.Metadata["task"], Coverage: req.Metadata["coverage"],
		QAAttemptID: req.Metadata["qa_attempt"], OperationalAttempt: req.Metadata["operational_attempt"], MapID: req.Metadata["map"], ShardID: req.Metadata["shard"],
		ArbiterGroupID: req.Metadata["arbiter_group"], EvidenceID: req.Metadata["evidence"], IssueID: req.Metadata["issue"], RepairRunID: req.Metadata["repair_run"], Cycle: req.Metadata["cycle"], Call: req.Metadata["call"],
		CallID: result.RunID, TraceID: req.TraceID, RunID: result.RunID, SessionID: result.SessionID, TurnID: result.TurnID, SessionAction: req.SessionAction, Status: result.Status,
		Provider: req.Provider, Model: req.Model, Variant: req.Metadata["variant"], Fallback: result.Policy.FinalTargetIndex > 0, FinalTargetIndex: result.Policy.FinalTargetIndex, Sandbox: req.Sandbox,
		PromptBytes: explanation.TotalBytes, SharedPrefixBytes: explanation.SharedPrefixBytes, StageSuffixBytes: explanation.StageSuffixBytes,
		SharedPrefixDigest: explanation.SharedPrefixDigest, CacheKey: explanation.CacheKey, CacheMode: req.Cache.Mode, CacheTransport: req.Metadata["prompt_cache_transport"],
		InputTokens: metricToken(result.Usage.InputTokensKnown, result.Usage.InputTokens), OutputTokens: metricToken(result.Usage.OutputTokensKnown, result.Usage.OutputTokens),
		ReasoningTokens: metricToken(result.Usage.ReasoningTokensKnown, result.Usage.ReasoningTokens), CacheReadTokens: metricToken(result.Usage.CacheReadTokensKnown, result.Usage.CacheReadTokens),
		CacheWriteTokens: metricToken(result.Usage.CacheWriteTokensKnown, result.Usage.CacheWriteTokens), TotalTokens: metricToken(result.Usage.TotalTokensKnown, result.Usage.TotalTokens),
		Turns: metricToken(result.Usage.TurnsKnown, result.Usage.Turns), StartedAt: result.StartedAt, FinishedAt: result.FinishedAt,
		ToolCalls: toolCalls, ToolCallsByKind: toolKinds, ToolCallCountExact: result.EventStats.Dropped == 0,
		RuntimeEvents: result.EventStats.Total, RetainedEvents: result.EventStats.Retained, DroppedEvents: result.EventStats.Dropped, RuntimeAttempts: len(result.Attempts), WarningCount: len(result.Warnings),
		PermissionMode: result.Permissions.Mode, PermissionDefault: result.Permissions.Default, UnsupportedTools: result.Permissions.UnsupportedCount, PermissionAudits: result.Permissions.AuditCount,
		DistinctFilesRead: contextMetrics.DistinctFilesRead, ReadBytes: contextMetrics.ReadBytes, RepeatedReads: contextMetrics.RepeatedReads, SearchCalls: contextMetrics.SearchCalls,
		Continuation: req.SessionAction == "continue", ParentRunID: req.ParentTraceID,
		RepairOf: req.Metadata["repair_of"], RetryOf: req.Metadata["retry_of"],
	}
	if len(result.Attempts) > 0 {
		actual := result.Attempts[len(result.Attempts)-1]
		if actual.Provider != "" {
			record.Provider = actual.Provider
		}
		if actual.Model != "" {
			record.Model = actual.Model
		}
	}
	if record.CallID == "" {
		record.CallID = fmt.Sprintf("runtime-call-%d", record.Sequence)
	}
	if record.RuntimeAttempts == 0 && (record.RunID != "" || record.Status != "") {
		record.RuntimeAttempts = 1
	}
	if record.RuntimeEvents == 0 {
		record.RuntimeEvents = int64(len(result.Events))
	}
	if record.RetainedEvents == 0 {
		record.RetainedEvents = len(result.Events)
	}
	if req.SessionAction == "continue" {
		record.ContinuationBytes = len(req.Prompt)
	}
	if !result.StartedAt.IsZero() && !result.FinishedAt.IsZero() {
		record.DurationMs = result.FinishedAt.Sub(result.StartedAt).Milliseconds()
	}
	if firstOutput > 0 {
		record.TimeToFirstOutputMs = firstOutput
	}
	if result.EstimatedCost != nil {
		record.CostAmount, record.CostCurrency, record.CostEstimated = result.EstimatedCost.Amount, result.EstimatedCost.Currency, result.EstimatedCost.Estimate
		record.CostSource = result.EstimatedCost.Source
	}
	if result.Error != nil {
		record.ErrorCategory = result.Error.Category
		record.ErrorDetail = qaSafeDiagnostic(result.Error.UserDetail)
	}
	metrics.Runs = append(metrics.Runs, record)
	metrics.UpdatedAt = s.now().UTC()
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	path, err := resolveSprintContained(s.root, sp, RuntimeMetricsRelPath(sp))
	if err != nil {
		return err
	}
	return atomicWriteFile(path, data)
}

func runtimeEventMetrics(result pruntime.Result) (map[string]int, int64) {
	kinds := map[string]int{}
	var firstOutput int64
	for _, event := range result.Events {
		kind := strings.ToLower(strings.TrimSpace(event.Kind))
		typeName := strings.ToLower(strings.TrimSpace(event.Type))
		if strings.Contains(kind, "tool") || strings.Contains(typeName, "tool_use") || strings.Contains(typeName, "tool.call") {
			name := "unknown"
			for _, key := range []string{"tool", "tool_name", "name"} {
				if value, ok := event.Payload[key].(string); ok && strings.TrimSpace(value) != "" {
					name = strings.ToLower(strings.TrimSpace(value))
					break
				}
			}
			kinds[name]++
		}
		if firstOutput == 0 && !result.StartedAt.IsZero() && !event.Time.IsZero() && (strings.Contains(kind, "text") || strings.Contains(typeName, "text") || event.Payload["content"] != nil) {
			firstOutput = event.Time.Sub(result.StartedAt).Milliseconds()
			if firstOutput < 0 {
				firstOutput = 0
			}
		}
	}
	if len(kinds) == 0 {
		kinds = nil
	}
	return kinds, firstOutput
}

func metricToken(known bool, value int64) RuntimeTokenMetric {
	return RuntimeTokenMetric{Known: known, Value: value}
}
