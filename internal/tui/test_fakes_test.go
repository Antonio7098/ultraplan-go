package tui

import (
	"context"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

type fakeUseCases struct {
	dashboardCalls int
	previewCalls   int
	lastContextErr error
	result         app.DashboardResult
	preview        app.ArtifactPreviewResult
	err            error
	validation     app.ValidationOperationResult
}

func (f *fakeUseCases) Validate(ctx context.Context, req app.ValidationRequest) (app.ValidationOperationResult, error) {
	f.lastContextErr = ctx.Err()
	return f.validation, f.err
}

func (f *fakeUseCases) PrepareOperation(ctx context.Context, req app.OperationRequest) (app.Confirmation, error) {
	return app.Confirmation{Request: req}, f.err
}

func (f *fakeUseCases) RunOperation(ctx context.Context, req app.OperationRequest, emit func(app.OperationEvent)) (app.OperationResult, error) {
	return app.OperationResult{State: app.OperationComplete}, f.err
}

func (f *fakeUseCases) Dashboard(ctx context.Context) (app.DashboardResult, error) {
	f.dashboardCalls++
	f.lastContextErr = ctx.Err()
	return f.result, f.err
}

func (f *fakeUseCases) PreviewArtifact(ctx context.Context, path string) (app.ArtifactPreviewResult, error) {
	f.previewCalls++
	f.lastContextErr = ctx.Err()
	f.preview.Path = path
	return f.preview, f.err
}
