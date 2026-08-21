package app

import (
	"context"

	"github.com/Antonio7098/ultraplan-go/internal/runcontrol"
)

type RunID = runcontrol.RunID
type RunQuery = runcontrol.Query
type RunPage = runcontrol.Page
type RunSnapshot = runcontrol.Snapshot
type RunEvent = runcontrol.Event
type RunLifecycle = runcontrol.Lifecycle
type RunTarget = runcontrol.Target
type RunCancellation = runcontrol.Cancellation
type RunHealthResult = runcontrol.Health
type RunOmission = runcontrol.Omission

var (
	ErrRunInvalidArgument   = runcontrol.ErrInvalidArgument
	ErrRunNotFound          = runcontrol.ErrNotFound
	ErrRunConflict          = runcontrol.ErrConflict
	ErrRunUnsupportedSchema = runcontrol.ErrUnsupportedSchema
)

// RunUseCases is the surface-neutral durable observation and command boundary.
// Its DTOs contain only sanitized facts owned by run control.
type RunUseCases interface {
	Runs(context.Context, runcontrol.Query) (runcontrol.Page, error)
	Run(context.Context, runcontrol.RunID) (runcontrol.Snapshot, error)
	RunEvents(context.Context, runcontrol.RunID, uint64, int) ([]runcontrol.Event, error)
	CancelRun(context.Context, runcontrol.RunID, string) (runcontrol.Snapshot, bool, error)
	RunHealth(context.Context) (runcontrol.Health, error)
}

type repositoryRunUseCases struct{ repository runcontrol.Repository }

func (u repositoryRunUseCases) Runs(ctx context.Context, query runcontrol.Query) (runcontrol.Page, error) {
	return u.repository.List(ctx, query)
}

func (u repositoryRunUseCases) Run(ctx context.Context, id runcontrol.RunID) (runcontrol.Snapshot, error) {
	return u.repository.Snapshot(ctx, id)
}

func (u repositoryRunUseCases) RunEvents(ctx context.Context, id runcontrol.RunID, after uint64, limit int) ([]runcontrol.Event, error) {
	return u.repository.Events(ctx, id, after, limit)
}

func (u repositoryRunUseCases) CancelRun(ctx context.Context, id runcontrol.RunID, reason string) (runcontrol.Snapshot, bool, error) {
	return u.repository.RequestCancellation(ctx, id, reason)
}

func (u repositoryRunUseCases) RunHealth(ctx context.Context) (runcontrol.Health, error) {
	return u.repository.Health(ctx)
}
