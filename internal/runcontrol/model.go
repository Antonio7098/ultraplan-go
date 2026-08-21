package runcontrol

import (
	"fmt"
	"strings"
	"time"
)

const (
	MaxTargetFieldBytes    = 512
	MaxSafeValueBytes      = 2048
	MaxEncodedEventBytes   = 16 << 10
	ProgressCoalesceWindow = 250 * time.Millisecond
)

type Lifecycle string

const (
	LifecycleAccepted         Lifecycle = "accepted"
	LifecycleQueued           Lifecycle = "queued"
	LifecycleRunning          Lifecycle = "running"
	LifecycleCancelling       Lifecycle = "cancelling"
	LifecycleSucceeded        Lifecycle = "succeeded"
	LifecycleFailed           Lifecycle = "failed"
	LifecycleCancelled        Lifecycle = "cancelled"
	LifecycleTimedOut         Lifecycle = "timed_out"
	LifecycleInterrupted      Lifecycle = "interrupted"
	LifecycleCleanupUncertain Lifecycle = "cleanup_uncertain"
	LifecyclePersistenceLost  Lifecycle = "persistence_degraded"
)

func (s Lifecycle) IsValid() bool {
	switch s {
	case LifecycleAccepted, LifecycleQueued, LifecycleRunning, LifecycleCancelling,
		LifecycleSucceeded, LifecycleFailed, LifecycleCancelled, LifecycleTimedOut,
		LifecycleInterrupted, LifecycleCleanupUncertain, LifecyclePersistenceLost:
		return true
	default:
		return false
	}
}

func (s Lifecycle) IsActive() bool {
	switch s {
	case LifecycleAccepted, LifecycleQueued, LifecycleRunning, LifecycleCancelling:
		return true
	default:
		return false
	}
}

func (s Lifecycle) IsTerminal() bool { return s.IsValid() && !s.IsActive() }

type Liveness string

const (
	LivenessUnknown          Liveness = "unknown"
	LivenessLive             Liveness = "live"
	LivenessStalled          Liveness = "stalled"
	LivenessOwnerUnreachable Liveness = "owner_unreachable"
	LivenessInterrupted      Liveness = "interrupted"
	LivenessCleanupUncertain Liveness = "cleanup_uncertain"
	LivenessTerminal         Liveness = "terminal"
)

func (s Liveness) IsValid() bool {
	switch s {
	case LivenessUnknown, LivenessLive, LivenessStalled, LivenessOwnerUnreachable,
		LivenessInterrupted, LivenessCleanupUncertain, LivenessTerminal:
		return true
	default:
		return false
	}
}

type CancellationState string

const (
	CancellationNone         CancellationState = "none"
	CancellationRequested    CancellationState = "requested"
	CancellationAcknowledged CancellationState = "acknowledged"
	CancellationUncertain    CancellationState = "uncertain"
)

func (s CancellationState) IsValid() bool {
	switch s {
	case CancellationNone, CancellationRequested, CancellationAcknowledged, CancellationUncertain:
		return true
	default:
		return false
	}
}

type TerminalOutcome string

const (
	TerminalSucceeded        TerminalOutcome = "succeeded"
	TerminalFailed           TerminalOutcome = "failed"
	TerminalCancelled        TerminalOutcome = "cancelled"
	TerminalTimedOut         TerminalOutcome = "timed_out"
	TerminalInterrupted      TerminalOutcome = "interrupted"
	TerminalCleanupUncertain TerminalOutcome = "cleanup_uncertain"
	TerminalPersistenceLost  TerminalOutcome = "persistence_degraded"
)

func (o TerminalOutcome) IsValid() bool {
	switch o {
	case TerminalSucceeded, TerminalFailed, TerminalCancelled, TerminalTimedOut,
		TerminalInterrupted, TerminalCleanupUncertain, TerminalPersistenceLost:
		return true
	default:
		return false
	}
}

func (o TerminalOutcome) Lifecycle() Lifecycle {
	return Lifecycle(o)
}

type EventType string

const (
	EventAccepted     EventType = "accepted"
	EventClaimed      EventType = "claimed"
	EventLifecycle    EventType = "lifecycle"
	EventProgress     EventType = "progress"
	EventMessage      EventType = "message"
	EventWarning      EventType = "warning"
	EventFinding      EventType = "finding"
	EventArtifact     EventType = "artifact"
	EventCancellation EventType = "cancellation"
	EventRecovery     EventType = "recovery"
	EventTerminal     EventType = "terminal"
	EventOmission     EventType = "omission"
)

func (t EventType) IsValid() bool {
	switch t {
	case EventAccepted, EventClaimed, EventLifecycle, EventProgress, EventMessage,
		EventWarning, EventFinding, EventArtifact, EventCancellation, EventRecovery,
		EventTerminal, EventOmission:
		return true
	default:
		return false
	}
}

type RecordState string

const (
	RecordFull      RecordState = "full"
	RecordCompacted RecordState = "compacted"
	RecordTombstone RecordState = "tombstone"
)

func (s RecordState) IsValid() bool {
	return s == RecordFull || s == RecordCompacted || s == RecordTombstone
}

// Target is a safe product-owned correlation. Values are identifiers or
// workspace-relative references, never unrestricted paths.
type Target struct {
	Kind      string `json:"kind"`
	Operation string `json:"operation"`
	Project   string `json:"project,omitempty"`
	Sprint    string `json:"sprint,omitempty"`
	Study     string `json:"study,omitempty"`
	Stage     string `json:"stage,omitempty"`
	Task      string `json:"task,omitempty"`
}

func (t Target) Validate() error {
	if strings.TrimSpace(t.Kind) == "" {
		return invalidField("target.kind", "is required")
	}
	if strings.TrimSpace(t.Operation) == "" {
		return invalidField("target.operation", "is required")
	}
	for name, value := range map[string]string{
		"kind": t.Kind, "operation": t.Operation, "project": t.Project,
		"sprint": t.Sprint, "study": t.Study, "stage": t.Stage, "task": t.Task,
	} {
		if len(value) > MaxTargetFieldBytes {
			return invalidField("target."+name, fmt.Sprintf("exceeds %d bytes", MaxTargetFieldBytes))
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return invalidField("target."+name, "contains a control character")
		}
	}
	return nil
}

type ProcessIdentity struct {
	HostDigest string `json:"host_digest,omitempty"`
	BootID     string `json:"boot_id,omitempty"`
	PID        int    `json:"pid,omitempty"`
	BirthToken string `json:"birth_token,omitempty"`
}

func (p ProcessIdentity) Validate() error {
	if p.PID < 0 {
		return invalidField("process.pid", "must not be negative")
	}
	for name, value := range map[string]string{"host_digest": p.HostDigest, "boot_id": p.BootID, "birth_token": p.BirthToken} {
		if len(value) > MaxSafeValueBytes || strings.ContainsAny(value, "\x00\r\n") {
			return invalidField("process."+name, "is not a bounded safe value")
		}
	}
	return nil
}

type Owner struct {
	ID      string          `json:"id"`
	Process ProcessIdentity `json:"process"`
}

func (o Owner) Validate() error {
	if strings.TrimSpace(o.ID) == "" || len(o.ID) > MaxSafeValueBytes || strings.ContainsAny(o.ID, "\x00\r\n") {
		return invalidField("owner.id", "must be a bounded opaque value")
	}
	return o.Process.Validate()
}

type Correlation struct {
	ProductRunID         string `json:"product_run_id,omitempty"`
	ProductTaskID        string `json:"product_task_id,omitempty"`
	RuntimeRunID         string `json:"runtime_run_id,omitempty"`
	AgentwrapRunID       string `json:"agentwrap_run_id,omitempty"`
	ProviderSessionID    string `json:"provider_session_id,omitempty"`
	ExternalHarnessRunID string `json:"external_harness_run_id,omitempty"`
}

func (c Correlation) Validate() error {
	for name, value := range map[string]string{
		"product_run_id": c.ProductRunID, "product_task_id": c.ProductTaskID,
		"runtime_run_id": c.RuntimeRunID, "agentwrap_run_id": c.AgentwrapRunID,
		"provider_session_id": c.ProviderSessionID, "external_harness_run_id": c.ExternalHarnessRunID,
	} {
		if len(value) > MaxSafeValueBytes || strings.ContainsAny(value, "\x00\r\n") {
			return invalidField("correlation."+name, "must be a bounded opaque value")
		}
	}
	return nil
}

type Cancellation struct {
	State          CancellationState `json:"state"`
	Reason         string            `json:"reason,omitempty"`
	RequestedAt    *time.Time        `json:"requested_at,omitempty"`
	AcknowledgedAt *time.Time        `json:"acknowledged_at,omitempty"`
}

type Terminal struct {
	Outcome    TerminalOutcome `json:"outcome"`
	Reason     string          `json:"reason,omitempty"`
	WonAt      time.Time       `json:"won_at"`
	ProposedBy string          `json:"proposed_by,omitempty"`
}

type Omission struct {
	Reason  string     `json:"reason"`
	Count   uint64     `json:"count"`
	FirstAt *time.Time `json:"first_at,omitempty"`
	LastAt  *time.Time `json:"last_at,omitempty"`
}

type Attempt struct {
	ID                AttemptID       `json:"id"`
	RunID             RunID           `json:"run_id"`
	Ordinal           uint64          `json:"ordinal"`
	Owner             Owner           `json:"owner"`
	FencingGeneration uint64          `json:"fencing_generation"`
	ClaimedAt         time.Time       `json:"claimed_at"`
	HeartbeatAt       time.Time       `json:"heartbeat_at"`
	LeaseExpiresAt    time.Time       `json:"lease_expires_at"`
	Correlation       Correlation     `json:"correlation,omitempty"`
	Outcome           TerminalOutcome `json:"outcome,omitempty"`
}

type Fence struct {
	RunID             RunID
	AttemptID         AttemptID
	OwnerID           string
	FencingGeneration uint64
}

func (f Fence) Validate() error {
	if err := f.RunID.Validate(); err != nil {
		return err
	}
	if err := f.AttemptID.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(f.OwnerID) == "" || f.FencingGeneration == 0 {
		return invalidField("fence", "requires owner_id and a positive generation")
	}
	return nil
}

type Event struct {
	RunID       RunID             `json:"run_id"`
	Sequence    uint64            `json:"sequence"`
	CommittedAt time.Time         `json:"committed_at"`
	Type        EventType         `json:"type"`
	AttemptID   AttemptID         `json:"attempt_id,omitempty"`
	Stage       string            `json:"stage,omitempty"`
	Task        string            `json:"task,omitempty"`
	Payload     map[string]string `json:"payload,omitempty"`
	Omission    *Omission         `json:"omission,omitempty"`
}

type Snapshot struct {
	RunID                  RunID        `json:"run_id"`
	Target                 Target       `json:"target"`
	Lifecycle              Lifecycle    `json:"lifecycle"`
	Liveness               Liveness     `json:"liveness"`
	RecordState            RecordState  `json:"record_state"`
	AcceptedAt             time.Time    `json:"accepted_at"`
	UpdatedAt              time.Time    `json:"updated_at"`
	StartedAt              *time.Time   `json:"started_at,omitempty"`
	FinishedAt             *time.Time   `json:"finished_at,omitempty"`
	CurrentAttemptID       AttemptID    `json:"current_attempt_id,omitempty"`
	LastSequence           uint64       `json:"last_sequence"`
	OldestRetainedSequence uint64       `json:"oldest_retained_sequence"`
	HistoryComplete        bool         `json:"history_complete"`
	OmissionTotal          uint64       `json:"omission_total"`
	Correlation            Correlation  `json:"correlation,omitempty"`
	ProductStatus          string       `json:"product_status,omitempty"`
	Cancellation           Cancellation `json:"cancellation"`
	Terminal               *Terminal    `json:"terminal,omitempty"`
}

func (s Snapshot) Validate() error {
	if err := s.RunID.Validate(); err != nil {
		return err
	}
	if err := s.Target.Validate(); err != nil {
		return err
	}
	if err := s.Correlation.Validate(); err != nil {
		return err
	}
	if !s.Lifecycle.IsValid() || !s.Liveness.IsValid() || !s.RecordState.IsValid() || !s.Cancellation.State.IsValid() {
		return invalidField("snapshot", "contains an unknown lifecycle, liveness, record, or cancellation state")
	}
	if s.AcceptedAt.IsZero() || s.UpdatedAt.Before(s.AcceptedAt) {
		return invalidField("snapshot.timestamps", "must start at acceptance and move forward")
	}
	if s.LastSequence == 0 && s.OldestRetainedSequence != 1 {
		return invalidField("snapshot.oldest_retained_sequence", "must be 1 before the first event")
	}
	if s.LastSequence > 0 && (s.OldestRetainedSequence == 0 || s.OldestRetainedSequence > s.LastSequence+1) {
		return invalidField("snapshot.sequence", "contains inconsistent replay boundaries")
	}
	if s.Lifecycle.IsTerminal() {
		if s.Terminal == nil || s.FinishedAt == nil || !s.Terminal.Outcome.IsValid() || s.Terminal.Outcome.Lifecycle() != s.Lifecycle {
			return invalidField("snapshot.terminal", "must match the terminal lifecycle and timestamp")
		}
		if s.Liveness != LivenessTerminal {
			return invalidField("snapshot.liveness", "must be terminal after a terminal outcome wins")
		}
	} else if s.Terminal != nil || s.FinishedAt != nil {
		return invalidField("snapshot.terminal", "must be absent while lifecycle is active")
	}
	return nil
}

type Query struct {
	Lifecycle  []Lifecycle
	TargetKind string
	Project    string
	Sprint     string
	Study      string
	Limit      int
	After      string
}

type Page struct {
	Runs       []Snapshot `json:"runs"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

type HealthStatus string

const (
	HealthOK       HealthStatus = "ok"
	HealthDegraded HealthStatus = "degraded"
	HealthFailed   HealthStatus = "failed"
)

type Health struct {
	Status                HealthStatus  `json:"status"`
	DatabasePath          string        `json:"database_path,omitempty"`
	JournalMode           string        `json:"journal_mode,omitempty"`
	Synchronous           string        `json:"synchronous,omitempty"`
	ForeignKeys           bool          `json:"foreign_keys"`
	BusyTimeout           time.Duration `json:"busy_timeout"`
	ActiveRuns            int           `json:"active_runs"`
	StalledRuns           int           `json:"stalled_runs"`
	CancellationUncertain int           `json:"cancellation_uncertain"`
	ReconciliationBacklog int           `json:"reconciliation_backlog"`
	OldestBacklogAge      time.Duration `json:"oldest_backlog_age"`
	Diagnostics           []string      `json:"diagnostics,omitempty"`
	StorageBytes          int64         `json:"storage_bytes"`
	SoftQuotaBytes        int64         `json:"soft_quota_bytes"`
	HardQuotaBytes        int64         `json:"hard_quota_bytes"`
	ReservedHeadroomBytes int64         `json:"reserved_headroom_bytes"`
	Metrics               LocalMetrics  `json:"metrics"`
}

// ReconciliationEvidence is the bounded, sanitized decision record exposed to
// diagnostics. Raw process details are intentionally not returned.
type ReconciliationEvidence struct {
	RunID      RunID     `json:"run_id"`
	AttemptID  AttemptID `json:"attempt_id,omitempty"`
	ObservedAt time.Time `json:"observed_at"`
	Action     string    `json:"action"`
	Decision   string    `json:"decision"`
	Evidence   string    `json:"evidence"`
}

const (
	DefaultFullHistory      = 7 * 24 * time.Hour
	DefaultTombstoneHistory = 30 * 24 * time.Hour
	DefaultHardQuotaBytes   = int64(512 << 20)
	ReservedHeadroomBytes   = int64(16 << 20)
	MaxRetainedEventsPerRun = 4096
	MaxRetainedBytesPerRun  = int64(16 << 20)
)

type RetentionPolicy struct {
	FullHistory      time.Duration
	TombstoneHistory time.Duration
	HardQuotaBytes   int64
}

type CompactionReport struct {
	CompactedRuns  int   `json:"compacted_runs"`
	TombstonedRuns int   `json:"tombstoned_runs"`
	DeletedRuns    int   `json:"deleted_runs"`
	DeletedEvents  int64 `json:"deleted_events"`
}

type Acceptance struct {
	RunID              RunID
	Target             Target
	Correlation        Correlation
	ProductStatus      string
	OperationAlias     string
	ConfirmationDigest string
}

type Claim struct {
	RunID       RunID
	AttemptID   AttemptID
	Owner       Owner
	Lease       time.Duration
	Correlation Correlation
}

type EventDraft struct {
	Type      EventType
	Stage     string
	Task      string
	Payload   map[string]string
	Omission  *Omission
	Lifecycle Lifecycle
}

type TerminalProposal struct {
	Outcome    TerminalOutcome
	Reason     string
	ProposedBy string
}

const (
	OwnerTickInterval      = time.Second
	HeartbeatInterval      = 5 * time.Second
	OwnerLeaseDuration     = 15 * time.Second
	ReconciliationInterval = 10 * time.Second
	ReconciliationGrace    = 45 * time.Second
	DefaultReconcileBatch  = 64
	MaximumReconcileBatch  = 256
)

type ReconcileOptions struct {
	Grace time.Duration
	Limit int
}

type ReconcileDecision struct {
	RunID     RunID     `json:"run_id"`
	AttemptID AttemptID `json:"attempt_id"`
	Action    string    `json:"action"`
	Decision  string    `json:"decision"`
}

type ReconcileReport struct {
	Scanned   int                 `json:"scanned"`
	Terminal  int                 `json:"terminal"`
	Stalled   int                 `json:"stalled"`
	Uncertain int                 `json:"uncertain"`
	Decisions []ReconcileDecision `json:"decisions,omitempty"`
}
