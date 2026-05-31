package study

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"ultraplan-go/internal/workspace"
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

	var tasks []TaskState
	for _, dimension := range dimensions {
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
			id := analysisTaskID(req.Study, dimension, source)
			outputPath := relPath(req.WorkspaceRoot, SourceReportPath(req.Study, source, dimension))
			tasks = append(tasks, TaskState{
				ID:           id,
				Kind:         TaskKindAnalysis,
				Status:       TaskStatusPending,
				Study:        req.Study.Name,
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
			ID:           synthesisTaskID(req.Study, dimension),
			Kind:         TaskKindSynthesis,
			Status:       TaskStatusPending,
			Study:        req.Study.Name,
			Dimension:    dimension.Number,
			DimensionRef: dimension.Ref(),
			OutputPath:   relPath(req.WorkspaceRoot, FinalReportPath(req.Study)),
			CreatedAt:    now,
			UpdatedAt:    now,
			Dependencies: deps,
		})
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return RunState{
		SchemaVersion: RunStateSchemaVersion,
		RunID:         runID,
		Study:         req.Study.Name,
		CreatedAt:     now,
		UpdatedAt:     now,
		Filters:       req.Filters,
		Config:        req.Config,
		Tasks:         tasks,
		Complete:      len(tasks) == 0,
	}, nil
}

func RunStatePath(study Study) string {
	return filepath.Join(study.Path, RunStateDirName, RunStateFileName)
}

func SummarizeRunState(state RunState, statePath string) StatusSummary {
	summary := StatusSummary{Total: len(state.Tasks), Complete: state.Complete, StatePath: statePath, RunID: state.RunID}
	for _, task := range state.Tasks {
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
	return summary
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
		case TaskStatusRunning, TaskStatusValidating, TaskStatusWaiting, TaskStatusRetrying:
			task.Status = TaskStatusPending
			task.UpdatedAt = now
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
				result = ValidateFinalReport(study)
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
