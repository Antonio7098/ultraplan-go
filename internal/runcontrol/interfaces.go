package runcontrol

import (
	"context"
	"time"
)

// Clock is injected so lifecycle tests do not depend on wall-clock timing.
type Clock interface {
	Now() time.Time
}

type WallClock struct{}

func (WallClock) Now() time.Time { return time.Now().UTC() }

// ProcessProbe resolves an exact process-birth identity. Implementations must
// report uncertainty instead of treating PID presence as ownership proof.
type ProcessProbe interface {
	Probe(context.Context, int) (ProcessIdentity, error)
}

type LogLevel string

const (
	LogDebug LogLevel = "debug"
	LogInfo  LogLevel = "info"
	LogWarn  LogLevel = "warn"
	LogError LogLevel = "error"
)

type LogField struct {
	Key   string
	Value string
}

type Logger interface {
	Log(context.Context, LogLevel, string, ...LogField)
}

// Notifier is a best-effort latency optimization. A notification never makes
// an event authoritative; observers always recover from Repository.Events.
type Notifier interface {
	Notify(RunID, uint64)
}

// Repository is the narrow durable authority used by the run-control service.
// Product workflow state and runtime supervision are intentionally absent.
type Repository interface {
	Accept(context.Context, Acceptance) (Snapshot, error)
	ResolveOperationAlias(context.Context, string) (Snapshot, error)
	Claim(context.Context, Claim) (Attempt, Snapshot, error)
	Snapshot(context.Context, RunID) (Snapshot, error)
	List(context.Context, Query) (Page, error)
	Append(context.Context, Fence, EventDraft) (Event, Snapshot, error)
	Heartbeat(context.Context, Fence, time.Duration) (Snapshot, error)
	RequestCancellation(context.Context, RunID, string) (Snapshot, bool, error)
	AcknowledgeCancellation(context.Context, Fence) (Snapshot, bool, error)
	ProposeTerminal(context.Context, Fence, TerminalProposal) (Snapshot, bool, error)
	Events(context.Context, RunID, uint64, int) ([]Event, error)
	Reconcile(context.Context, ProcessProbe, ReconcileOptions) (ReconcileReport, error)
	Compact(context.Context, int) (CompactionReport, error)
	Health(context.Context) (Health, error)
	Close() error
}

// Control is the adapter-neutral service capability. Keeping this interface
// separate permits app composition to decorate durable operations without
// exposing SQL or transport details.
type Control interface {
	Repository
}
