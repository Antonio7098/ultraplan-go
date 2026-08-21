package study

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type NewRunStateRequest struct {
	WorkspaceRoot string
	Study         Study
	Sources       []Source
	Dimensions    []Dimension
	RunID         string
	Now           time.Time
	Filters       RunFilters
	Config        ConfigSummary
}

func NewRunState(req NewRunStateRequest) (RunState, error) {
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	runID := strings.TrimSpace(req.RunID)
	if runID == "" {
		runID = fmt.Sprintf("run-%s", now.UTC().Format("20060102T150405Z"))
	}

	dimensions := append([]Dimension(nil), req.Dimensions...)
	sort.Slice(dimensions, func(i, j int) bool {
		if dimensions[i].Number == dimensions[j].Number {
			return dimensions[i].Slug < dimensions[j].Slug
		}
		return dimensions[i].Number < dimensions[j].Number
	})

	sources := append([]Source(nil), req.Sources...)
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Name == sources[j].Name {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].Name < sources[j].Name
	})

	tasks, fingerprint := buildCurrentTaskGraph(req.WorkspaceRoot, req.Study, sources, dimensions, now)
	return RunState{
		SchemaVersion:            RunStateSchemaVersion,
		RunID:                    runID,
		Study:                    req.Study.Name,
		CreatedAt:                now,
		UpdatedAt:                now,
		Filters:                  req.Filters,
		Config:                   req.Config,
		ApplicabilityFingerprint: fingerprint,
		Tasks:                    tasks,
		Complete:                 len(tasks) == 0,
	}, nil
}

func ReconcileRunState(state *RunState, workspaceRoot string, study Study, sources []Source, dimensions []Dimension, now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	current, fingerprint := buildCurrentTaskGraph(workspaceRoot, study, sources, dimensions, now)
	existing := make(map[string]TaskState, len(state.Tasks))
	for _, task := range state.Tasks {
		existing[task.ID] = task
	}
	for i := range current {
		if prior, ok := existing[current[i].ID]; ok {
			expected := current[i]
			current[i] = prior
			current[i].Dimension = expected.Dimension
			current[i].DimensionRef = expected.DimensionRef
			current[i].Source = expected.Source
			current[i].SourceKind = expected.SourceKind
			current[i].OutputPath = expected.OutputPath
			current[i].Dependencies = expected.Dependencies
			if current[i].Kind == TaskKindSynthesis && dependenciesChanged(prior.Dependencies, current[i].Dependencies) && prior.Status == TaskStatusCompleted {
				current[i].Status = TaskStatusPending
				current[i].CompletedAt = nil
				current[i].Validation = nil
				current[i].LastError = nil
				current[i].UpdatedAt = now
			}
		}
	}
	sort.Slice(current, func(i, j int) bool { return current[i].ID < current[j].ID })
	state.Tasks = current
	state.ApplicabilityFingerprint = fingerprint
	state.UpdatedAt = now
	state.Complete = allTasksComplete(*state)
}

func buildCurrentTaskGraph(workspaceRoot string, study Study, sources []Source, dimensions []Dimension, now time.Time) ([]TaskState, string) {
	dimensions = append([]Dimension(nil), dimensions...)
	sort.Slice(dimensions, func(i, j int) bool {
		if dimensions[i].Number == dimensions[j].Number {
			return dimensions[i].Slug < dimensions[j].Slug
		}
		return dimensions[i].Number < dimensions[j].Number
	})
	sources = append([]Source(nil), sources...)
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].Name == sources[j].Name {
			return sources[i].Kind < sources[j].Kind
		}
		return sources[i].Name < sources[j].Name
	})
	var tasks []TaskState
	var fingerprint strings.Builder
	for _, dimension := range dimensions {
		fmt.Fprintf(&fingerprint, "dimension:%s:%s\n", dimension.Number, dimension.Slug)
		applicable := GetApplicableSources(sources, dimension)
		sort.Slice(applicable, func(i, j int) bool {
			if applicable[i].Name == applicable[j].Name {
				return applicable[i].Kind < applicable[j].Kind
			}
			return applicable[i].Name < applicable[j].Name
		})
		if len(applicable) == 0 {
			continue
		}
		var deps []SynthesisDependency
		for _, source := range applicable {
			fmt.Fprintf(&fingerprint, "source:%s:%s:%s\n", dimension.Number, source.Name, source.Kind)
			id := analysisTaskID(study, dimension, source)
			outputPath := relPath(workspaceRoot, SourceReportPath(study, source, dimension))
			tasks = append(tasks, TaskState{
				ID:           id,
				Kind:         TaskKindAnalysis,
				Status:       TaskStatusPending,
				Study:        study.Name,
				Dimension:    dimension.Number,
				DimensionRef: dimension.Ref(),
				Source:       source.Name,
				SourceKind:   source.Kind,
				OutputPath:   outputPath,
				CreatedAt:    now,
				UpdatedAt:    now,
			})
			deps = append(deps, SynthesisDependency{
				TaskID:     id,
				Source:     source.Name,
				SourceKind: source.Kind,
				ReportPath: outputPath,
			})
		}
		sort.Slice(deps, func(i, j int) bool { return deps[i].TaskID < deps[j].TaskID })
		tasks = append(tasks, TaskState{
			ID:           synthesisTaskID(study, dimension),
			Kind:         TaskKindSynthesis,
			Status:       TaskStatusPending,
			Study:        study.Name,
			Dimension:    dimension.Number,
			DimensionRef: dimension.Ref(),
			OutputPath:   relPath(workspaceRoot, FinalReportPath(study, dimension)),
			CreatedAt:    now,
			UpdatedAt:    now,
			Dependencies: deps,
		})
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	sum := sha256.Sum256([]byte(fingerprint.String()))
	return tasks, "sha256:" + hex.EncodeToString(sum[:])
}

func dependenciesChanged(a, b []SynthesisDependency) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

func RunStatePath(study Study) string {
	return filepath.Join(study.Path, RunStateDirName, RunStateFileName)
}

func SummarizeRunState(state RunState, statePath string) StatusSummary {
	summary := SummarizeRunStateCounts(state, statePath)
	summary.Tasks = append([]TaskState(nil), state.Tasks...)
	return summary
}

// SummarizeRetries aggregates retry activity across tasks. A retry continued
// the same agent session when a compatible durable checkpoint exists without
// recorded continue failures; otherwise the retry started a fresh session.
func SummarizeRetries(state RunState) RetrySummary {
	var summary RetrySummary
	for _, task := range state.Tasks {
		retries := task.Attempts - 1
		if retries < 0 {
			retries = 0
		}
		if retries > 0 {
			summary.RetriedTasks++
			summary.TotalRetries += retries
			if TaskSessionContinued(task) {
				summary.SameSession++
			} else {
				summary.FreshSession++
			}
		}
		if task.Status == TaskStatusRetrying {
			summary.Waiting++
		}
		if task.RetryAfter != nil && (summary.NextRetryAt == nil || task.RetryAfter.Before(*summary.NextRetryAt)) {
			next := *task.RetryAfter
			summary.NextRetryAt = &next
		}
	}
	return summary
}

// TaskSessionContinued reports whether a task's retries continued the same
// agent session: a durable checkpoint exists without recorded continue
// failures.
func TaskSessionContinued(task TaskState) bool {
	return task.Session != nil && task.Session.SessionID != "" && task.Session.ContinueFailures == 0
}

func SummarizeRunStateCounts(state RunState, statePath string) StatusSummary {
	summary := StatusSummary{Total: len(state.Tasks), Complete: state.Complete, StatePath: statePath, RunID: state.RunID}
	for _, task := range state.Tasks {
		countTaskStatus(&summary, task)
	}
	return summary
}

func countTaskStatus(summary *StatusSummary, task TaskState) {
	switch task.Status {
	case TaskStatusPending:
		summary.Pending++
	case TaskStatusRunning:
		summary.Running++
		summary.Active++
	case TaskStatusValidating:
		summary.Validating++
		summary.Active++
	case TaskStatusCompleted:
		summary.Completed++
	case TaskStatusFailed:
		summary.Failed++
	case TaskStatusCancelled:
		summary.Cancelled++
	case TaskStatusSkipped:
		summary.Skipped++
	case TaskStatusWaiting:
		summary.Waiting++
		summary.Active++
	case TaskStatusRetrying:
		summary.Retrying++
		summary.Active++
	}
	if task.RetryAfter != nil {
		summary.RetryCount++
		if summary.NextRetryAt == nil || task.RetryAfter.Before(*summary.NextRetryAt) {
			next := *task.RetryAfter
			summary.NextRetryAt = &next
		}
	}
}

func ResumeValidateRunState(state *RunState, study Study, sources []Source, dimensions []Dimension, now time.Time) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	sourceByKey := map[string]Source{}
	for _, source := range sources {
		sourceByKey[sourceKey(source.Name, source.Kind)] = source
	}
	dimensionByRef := map[string]Dimension{}
	for _, dimension := range dimensions {
		dimensionByRef[dimension.Ref()] = dimension
		dimensionByRef[dimension.Number] = dimension
	}
	for i := range state.Tasks {
		task := &state.Tasks[i]
		switch task.Status {
		case TaskStatusRunning, TaskStatusValidating, TaskStatusWaiting, TaskStatusCancelled:
			task.Status = TaskStatusPending
			task.RetryAfter = nil
			task.UpdatedAt = now
		case TaskStatusRetrying:
			if task.RetryAfter == nil || !task.RetryAfter.After(now) {
				task.Status = TaskStatusPending
				task.RetryAfter = nil
				task.UpdatedAt = now
			}
		case TaskStatusFailed:
			if task.RetryAfter != nil {
				if task.RetryAfter.After(now) {
					task.Status = TaskStatusRetrying
				} else {
					task.Status = TaskStatusPending
					task.RetryAfter = nil
				}
				task.UpdatedAt = now
			}
		case TaskStatusCompleted:
			var result ValidationResult
			switch task.Kind {
			case TaskKindAnalysis:
				source, sourceOK := sourceByKey[sourceKey(task.Source, task.SourceKind)]
				dimension, dimensionOK := dimensionByRef[task.DimensionRef]
				if !dimensionOK {
					dimension, dimensionOK = dimensionByRef[task.Dimension]
				}
				if !sourceOK || !dimensionOK {
					task.Status = TaskStatusFailed
					task.LastError = &TaskError{Code: "validation.reference", Message: "completed task references unknown source or dimension"}
					task.UpdatedAt = now
					continue
				}
				result = ValidateSourceReport(study, source, dimension)
			case TaskKindSynthesis:
				dimension, dimensionOK := dimensionByRef[task.DimensionRef]
				if !dimensionOK {
					dimension, dimensionOK = dimensionByRef[task.Dimension]
				}
				if !dimensionOK {
					task.Status = TaskStatusFailed
					task.LastError = &TaskError{Code: "validation.reference", Message: "completed task references unknown dimension"}
					task.UpdatedAt = now
					continue
				}
				result = ValidateFinalReport(study, dimension)
			default:
				continue
			}
			summary := validationSummary(result, now)
			task.Validation = &summary
			if result.Status != ValidationStatusPassed {
				task.Status = TaskStatusFailed
				task.LastError = &TaskError{Code: "validation.report", Message: summary.Message, Path: summary.Path}
			}
			task.UpdatedAt = now
		}
	}
	state.Complete = true
	for _, task := range state.Tasks {
		if task.Status != TaskStatusCompleted && task.Status != TaskStatusSkipped {
			state.Complete = false
			break
		}
	}
	state.UpdatedAt = now
}

func validationSummary(result ValidationResult, now time.Time) ValidationSummary {
	summary := ValidationSummary{Status: result.Status, CheckedAt: now, Path: result.Path}
	for _, check := range result.Checks {
		switch check.Status {
		case ValidationStatusPassed:
			summary.PassedChecks++
		case ValidationStatusFailed:
			summary.FailedChecks++
		}
	}
	if result.Err != nil {
		summary.Message = result.Err.Error()
	}
	return summary
}

func analysisTaskID(study Study, dimension Dimension, source Source) string {
	return strings.Join([]string{
		string(TaskKindAnalysis),
		slugID(study.Name),
		dimension.Number,
		slugID(dimension.Slug),
		slugID(source.Name),
		string(source.Kind),
	}, ":")
}

func synthesisTaskID(study Study, dimension Dimension) string {
	return strings.Join([]string{string(TaskKindSynthesis), slugID(study.Name), dimension.Number, slugID(dimension.Slug)}, ":")
}

func slugID(value string) string {
	value = normalizeSlug(value)
	if value == "" {
		return "none"
	}
	return value
}

func relPath(root, path string) string {
	if root == "" {
		return filepath.ToSlash(filepath.Clean(path))
	}
	return workspace.Rel(root, path)
}

func sourceKey(name string, kind SourceKind) string {
	return name + "\x00" + string(kind)
}
