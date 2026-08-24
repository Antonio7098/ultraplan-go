package study

import (
	"context"
	"fmt"

	"github.com/Antonio7098/ultraplan-go/internal/platform/gitpublish"
)

func (s Service) publishExecution(ctx context.Context, result ExecutionResult, extraPaths ...string) (ExecutionResult, error) {
	if s.publisher == nil || result.Status != ExecutionStatusCompleted {
		return result, nil
	}
	paths := append([]string{result.OutputPath}, extraPaths...)
	subject := result.Dimension.Ref()
	if result.Source.Name != "" {
		subject += "/" + result.Source.Name
	}
	publication, err := s.publisher.Publish(ctx, gitpublish.Request{
		Root: result.Study.Path, Paths: paths,
		Message:  fmt.Sprintf("ultraplan: study %s complete %s %s", result.Study.Name, result.TaskKind, subject),
		Identity: fmt.Sprintf("study/%s/%s/%s", result.Study.Name, result.TaskKind, subject),
	})
	if !(publication.Repository == "" && publication.Skipped) {
		result.Publications = append(result.Publications, publication)
	}
	return result, err
}

func (s Service) publishRunAllSummary(ctx context.Context, result RunAllResult) (RunAllResult, error) {
	if s.publisher == nil || result.Status != RunAllStatusCompleted {
		return result, nil
	}
	publication, err := s.publisher.Publish(ctx, gitpublish.Request{
		Root: result.Study.Path, Paths: []string{result.SummaryPath},
		Message:  fmt.Sprintf("ultraplan: study %s complete run-all", result.Study.Name),
		Identity: fmt.Sprintf("study/%s/run-all", result.Study.Name),
	})
	if !(publication.Repository == "" && publication.Skipped) {
		result.Publications = append(result.Publications, publication)
	}
	return result, err
}

func (s Service) publishRunLoopState(ctx context.Context, study Study) ([]gitpublish.Result, error) {
	if s.publisher == nil {
		return nil, nil
	}
	publication, err := s.publisher.Publish(ctx, gitpublish.Request{
		Root: study.Path,
		Paths: []string{
			RunStatePath(study),
			RunHistoryPath(study),
			RunHistorySummaryPath(study),
		},
		Message:  fmt.Sprintf("ultraplan: study %s update run state", study.Name),
		Identity: fmt.Sprintf("study/%s/run-state", study.Name),
	})
	if publication.Repository == "" && publication.Skipped {
		return nil, err
	}
	return []gitpublish.Result{publication}, err
}
