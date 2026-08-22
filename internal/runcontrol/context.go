package runcontrol

import "context"

type parentRunContextKey struct{}

// WithParentRun records the durable parent run for child work started with ctx.
func WithParentRun(ctx context.Context, runID RunID) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if runID == "" {
		return ctx
	}
	return context.WithValue(ctx, parentRunContextKey{}, runID)
}

// ParentRun returns the durable parent run carried by ctx.
func ParentRun(ctx context.Context) RunID {
	if ctx == nil {
		return ""
	}
	runID, _ := ctx.Value(parentRunContextKey{}).(RunID)
	return runID
}
