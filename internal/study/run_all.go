package study

import (
	"context"
	"fmt"
	"sync"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type analysisTask struct {
	dimension Dimension
	source    Source
}

func (s Service) RunAll(ctx context.Context, req RunAllRequest) (RunAllResult, error) {
	if req.Parallelism < 1 {
		return RunAllResult{}, fmt.Errorf("parallelism must be at least 1")
	}
	listing, err := s.ListStudy(req.StudyRef)
	if err != nil {
		return RunAllResult{}, err
	}
	dimensions, err := resolveDimensions(listing.Dimensions, req.DimensionRefs)
	if err != nil {
		return RunAllResult{}, err
	}
	sources, err := resolveSources(listing.Sources, req.SourceRefs)
	if err != nil {
		return RunAllResult{}, err
	}
	result := RunAllResult{Study: listing.Study, Parallelism: req.Parallelism, SummaryPath: SummaryPath(listing.Study)}
	for _, group := range dimensionExecutionGroups(dimensions, listing.DimensionOrder) {
		groupResult := s.runAllDimensionGroup(ctx, req, listing.Study, group, sources)
		result.Analysis = append(result.Analysis, groupResult.Analysis...)
		result.Synthesis = append(result.Synthesis, groupResult.Synthesis...)
	}

	summary, err := WriteSummary(listing.Study, dimensions, sources)
	if err != nil {
		result.Warnings = append(result.Warnings, RunAllWarning{Path: result.SummaryPath, Message: safeError(err)})
	} else {
		result.SummaryResult = summary
		for _, warning := range summary.Warnings {
			result.Warnings = append(result.Warnings, RunAllWarning{Path: warning.Path, Message: warning.Message})
		}
	}
	result.Counts = countRunAll(result)
	result.Status = statusRunAll(result, ctx.Err() != nil)
	return s.publishRunAllSummary(ctx, result)
}

func (s Service) runAllDimensionGroup(ctx context.Context, req RunAllRequest, study Study, dimensions []Dimension, sources []Source) RunAllResult {
	var result RunAllResult
	tasks := buildAnalysisTasks(dimensions, sources)
	result.Analysis = make([]ExecutionResult, len(tasks))
	taskCh := make(chan int)
	var wg sync.WaitGroup
	for i := 0; i < req.Parallelism; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range taskCh {
				task := tasks[idx]
				if ctx.Err() != nil {
					result.Analysis[idx] = pendingAnalysisResult(study, task)
					continue
				}
				onEvent := func(event runtimepkg.Event) {
					if req.Progress != nil {
						req.Progress(RunAllProgress{TaskKind: TaskKindAnalysis, DimensionRef: task.dimension.Ref(), SourceRef: task.source.Name, Event: event})
					}
				}
				res, err := s.RunAnalysis(ctx, ExecutionRequest{StudyRef: study.Name, DimensionRef: task.dimension.Ref(), SourceRef: task.source.Name, Model: req.Model, OnEvent: onEvent})
				if err != nil {
					res = failedAnalysisResult(study, task, err)
				}
				result.Analysis[idx] = res
			}
		}()
	}
	for idx := range tasks {
		if ctx.Err() != nil {
			result.Analysis[idx] = pendingAnalysisResult(study, tasks[idx])
			continue
		}
		taskCh <- idx
	}
	close(taskCh)
	wg.Wait()

	for _, dimension := range dimensions {
		if ctx.Err() != nil {
			result.Synthesis = append(result.Synthesis, ExecutionResult{Status: ExecutionStatusCancelled, TaskKind: TaskKindSynthesis, Study: study, Dimension: dimension, OutputPath: FinalReportPath(study, dimension)})
			continue
		}
		if !hasAnalysisForDimension(result.Analysis, dimension) {
			continue
		}
		if blockers := synthesisBlockers(result.Analysis, dimension); len(blockers) > 0 {
			result.Synthesis = append(result.Synthesis, ExecutionResult{Status: ExecutionStatusPreflightBlocked, TaskKind: TaskKindSynthesis, Study: study, Dimension: dimension, OutputPath: FinalReportPath(study, dimension), Blockers: blockers})
			continue
		}
		onEvent := func(event runtimepkg.Event) {
			if req.Progress != nil {
				req.Progress(RunAllProgress{TaskKind: TaskKindSynthesis, DimensionRef: dimension.Ref(), Event: event})
			}
		}
		res, err := s.Synthesize(ctx, SynthesisRequest{StudyRef: study.Name, DimensionRef: dimension.Ref(), SourceRefs: selectedSourceNames(sources), Model: req.Model, OnEvent: onEvent})
		if err != nil {
			res = ExecutionResult{Status: ExecutionStatusRuntimeFailed, TaskKind: TaskKindSynthesis, Study: study, Dimension: dimension, OutputPath: FinalReportPath(study, dimension), RuntimeError: safeError(err), RuntimeErr: err}
		}
		result.Synthesis = append(result.Synthesis, res)
	}
	return result
}

func dimensionExecutionGroups(selected, priority []Dimension) [][]Dimension {
	selectedByRef := make(map[string]Dimension, len(selected))
	for _, dimension := range selected {
		selectedByRef[dimension.Ref()] = dimension
	}
	groups := make([][]Dimension, 0, len(priority)+1)
	prioritized := make(map[string]bool, len(priority))
	for _, dimension := range priority {
		selectedDimension, ok := selectedByRef[dimension.Ref()]
		if !ok {
			continue
		}
		groups = append(groups, []Dimension{selectedDimension})
		prioritized[dimension.Ref()] = true
	}
	var remaining []Dimension
	for _, dimension := range selected {
		if !prioritized[dimension.Ref()] {
			remaining = append(remaining, dimension)
		}
	}
	if len(remaining) > 0 {
		groups = append(groups, remaining)
	}
	return groups
}

func selectedSourceNames(sources []Source) []string {
	names := make([]string, 0, len(sources))
	for _, source := range sources {
		names = append(names, source.Name)
	}
	return names
}

func hasAnalysisForDimension(results []ExecutionResult, dimension Dimension) bool {
	for _, result := range results {
		if result.Dimension.Ref() == dimension.Ref() {
			return true
		}
	}
	return false
}

func resolveDimensions(all []Dimension, refs []string) ([]Dimension, error) {
	if len(refs) == 0 {
		return append([]Dimension(nil), all...), nil
	}
	out := make([]Dimension, 0, len(refs))
	for _, ref := range refs {
		d, err := ResolveDimension(all, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

func resolveSources(all []Source, refs []string) ([]Source, error) {
	if len(refs) == 0 {
		return append([]Source(nil), all...), nil
	}
	out := make([]Source, 0, len(refs))
	for _, ref := range refs {
		s, err := ResolveSource(all, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

func buildAnalysisTasks(dimensions []Dimension, sources []Source) []analysisTask {
	var tasks []analysisTask
	for _, dimension := range dimensions {
		for _, source := range sources {
			if !SourceAppliesToDimension(source, dimension) {
				continue
			}
			tasks = append(tasks, analysisTask{dimension: dimension, source: source})
		}
	}
	return tasks
}

func pendingAnalysisResult(study Study, task analysisTask) ExecutionResult {
	return ExecutionResult{Status: ExecutionStatusCancelled, TaskKind: TaskKindAnalysis, Study: study, Dimension: task.dimension, Source: task.source, OutputPath: SourceReportPath(study, task.source, task.dimension)}
}

func failedAnalysisResult(study Study, task analysisTask, err error) ExecutionResult {
	return ExecutionResult{Status: ExecutionStatusRuntimeFailed, TaskKind: TaskKindAnalysis, Study: study, Dimension: task.dimension, Source: task.source, OutputPath: SourceReportPath(study, task.source, task.dimension), RuntimeError: safeError(err), RuntimeErr: err}
}

func synthesisBlockers(results []ExecutionResult, dimension Dimension) []string {
	var blockers []string
	for _, result := range results {
		if result.Dimension.Ref() != dimension.Ref() {
			continue
		}
		if result.Status != ExecutionStatusCompleted {
			blockers = append(blockers, result.OutputPath)
		}
	}
	return blockers
}

func countRunAll(result RunAllResult) RunAllCounts {
	var counts RunAllCounts
	for _, item := range append(append([]ExecutionResult{}, result.Analysis...), result.Synthesis...) {
		switch item.Status {
		case ExecutionStatusCompleted:
			counts.Completed++
		case ExecutionStatusSkipped, ExecutionStatusPreflightBlocked:
			counts.Skipped++
		case ExecutionStatusCancelled:
			counts.Pending++
		default:
			counts.Failed++
		}
	}
	return counts
}

func statusRunAll(result RunAllResult, cancelled bool) RunAllStatus {
	if cancelled {
		return RunAllStatusCancelled
	}
	if result.Counts.Failed == 0 && result.Counts.Pending == 0 && result.Counts.Skipped == 0 {
		return RunAllStatusCompleted
	}
	if result.Counts.Completed > 0 && (result.Counts.Failed > 0 || result.Counts.Skipped > 0 || result.Counts.Pending > 0) {
		return RunAllStatusPartial
	}
	for _, item := range append(append([]ExecutionResult{}, result.Analysis...), result.Synthesis...) {
		if item.Status == ExecutionStatusRuntimeFailed {
			return RunAllStatusRuntimeFailed
		}
	}
	return RunAllStatusValidationFailed
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
