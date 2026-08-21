package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
)

const (
	runtimeMetricsSchemaVersion = 1
	runtimeMetricsFileName      = ".runtime-metrics.json"
	maxRuntimeMetricRecords     = 512
)

type RuntimeTokenMetric struct {
	Known bool  `json:"known"`
	Value int64 `json:"value,omitempty"`
}

type SprintRuntimeMetric struct {
	Stage              PlanningStage      `json:"stage"`
	Operation          string             `json:"operation,omitempty"`
	Task               string             `json:"task,omitempty"`
	Coverage           string             `json:"coverage,omitempty"`
	RunID              string             `json:"run_id,omitempty"`
	SessionID          string             `json:"session_id,omitempty"`
	Status             string             `json:"status,omitempty"`
	Provider           string             `json:"provider,omitempty"`
	Model              string             `json:"model,omitempty"`
	PromptBytes        int                `json:"prompt_bytes"`
	SharedPrefixBytes  int                `json:"shared_prefix_bytes,omitempty"`
	StageSuffixBytes   int                `json:"stage_suffix_bytes"`
	SharedPrefixDigest string             `json:"shared_prefix_sha256,omitempty"`
	CacheKey           string             `json:"cache_key,omitempty"`
	InputTokens        RuntimeTokenMetric `json:"input_tokens"`
	OutputTokens       RuntimeTokenMetric `json:"output_tokens"`
	ReasoningTokens    RuntimeTokenMetric `json:"reasoning_tokens"`
	CacheReadTokens    RuntimeTokenMetric `json:"cache_read_tokens"`
	CacheWriteTokens   RuntimeTokenMetric `json:"cache_write_tokens"`
	TotalTokens        RuntimeTokenMetric `json:"total_tokens"`
	Turns              RuntimeTokenMetric `json:"turns"`
	CostAmount         float64            `json:"cost_amount,omitempty"`
	CostCurrency       string             `json:"cost_currency,omitempty"`
	CostEstimated      bool               `json:"cost_estimated,omitempty"`
	StartedAt          time.Time          `json:"started_at,omitempty"`
	FinishedAt         time.Time          `json:"finished_at,omitempty"`
	ErrorCategory      string             `json:"error_category,omitempty"`
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
	if metrics.SchemaVersion != runtimeMetricsSchemaVersion || metrics.Project != sp.Project || metrics.Sprint != sp.Slug {
		return SprintRuntimeMetrics{}, fmt.Errorf("invalid sprint runtime metrics identity")
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
	result, runErr := s.runtime.StartRun(ctx, req)
	if metricErr := s.recordRuntimeMetric(sp, stage, req, result); metricErr != nil {
		result.Warnings = append(result.Warnings, "runtime metrics were not persisted: "+safeError(metricErr))
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
	record := SprintRuntimeMetric{
		Stage: stage, Operation: req.Metadata["operation"], Task: req.Metadata["task"], Coverage: req.Metadata["coverage"],
		RunID: result.RunID, SessionID: result.SessionID, Status: result.Status, Provider: req.Provider, Model: req.Model,
		PromptBytes: explanation.TotalBytes, SharedPrefixBytes: explanation.SharedPrefixBytes, StageSuffixBytes: explanation.StageSuffixBytes,
		SharedPrefixDigest: explanation.SharedPrefixDigest, CacheKey: explanation.CacheKey,
		InputTokens: metricToken(result.Usage.InputTokensKnown, result.Usage.InputTokens), OutputTokens: metricToken(result.Usage.OutputTokensKnown, result.Usage.OutputTokens),
		ReasoningTokens: metricToken(result.Usage.ReasoningTokensKnown, result.Usage.ReasoningTokens), CacheReadTokens: metricToken(result.Usage.CacheReadTokensKnown, result.Usage.CacheReadTokens),
		CacheWriteTokens: metricToken(result.Usage.CacheWriteTokensKnown, result.Usage.CacheWriteTokens), TotalTokens: metricToken(result.Usage.TotalTokensKnown, result.Usage.TotalTokens),
		Turns: metricToken(result.Usage.TurnsKnown, result.Usage.Turns), StartedAt: result.StartedAt, FinishedAt: result.FinishedAt,
	}
	if result.EstimatedCost != nil {
		record.CostAmount, record.CostCurrency, record.CostEstimated = result.EstimatedCost.Amount, result.EstimatedCost.Currency, result.EstimatedCost.Estimate
	}
	if result.Error != nil {
		record.ErrorCategory = result.Error.Category
	}
	metrics.Runs = append(metrics.Runs, record)
	if len(metrics.Runs) > maxRuntimeMetricRecords {
		metrics.Runs = append([]SprintRuntimeMetric(nil), metrics.Runs[len(metrics.Runs)-maxRuntimeMetricRecords:]...)
	}
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

func metricToken(known bool, value int64) RuntimeTokenMetric {
	return RuntimeTokenMetric{Known: known, Value: value}
}
