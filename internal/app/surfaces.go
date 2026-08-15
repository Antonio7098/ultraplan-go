package app

import (
	"context"
	"io"
)

// ServeRunOptions is the immutable handoff from command preflight to the web
// transport. The query capability intentionally contains no workflow mutations.
type ServeRunOptions struct {
	Listen      string
	OpenBrowser bool
	UseCases    WebQueries
	Stdout      io.Writer
	Diagnostics io.Writer
}

// WebRunner is constructed at the process composition root. Keeping the
// implementation outside app avoids an app/web package cycle.
type WebRunner func(context.Context, ServeRunOptions) error
