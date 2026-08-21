// Package runtime provides UltraPlan's generic agent runtime boundary.
package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	stdruntime "runtime"
	"time"

	"github.com/Antonio7098/agentwrap"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
)

type Request struct {
	Prompt        string
	PromptRef     PromptReference
	TraceID       string
	ParentTraceID string
	WorkDir       string
	SessionID     string
	SessionAction string
	Provider      string
	Model         string
	Timeout       time.Duration
	Metadata      map[string]string
	RequireHealth []string
	RequireCaps   []string
	Sandbox       string
	Permissions   string
	Policy        PermissionPolicy
	Validation    *agentwrap.ValidationSpec
	OnEvent       func(Event)
	Cache         CacheDirective
}

// CacheDirective carries product-owned prompt boundary semantics to runtime
// adapters. The current OpenCode adapter preserves these values as request
// metadata; adapters with provider-native support can apply them directly.
type CacheDirective struct {
	Key             string
	BreakpointBytes int
	PrefixDigest    string
	Mode            string
}

// PromptReference identifies the exact product-owned prompt sent to a runtime.
// It intentionally carries identity rather than prompt contents.
type PromptReference struct {
	ID        string
	Version   string
	OwnerKind string
	OwnerID   string
	Purpose   string
	Checksum  string
}

type PermissionPolicy struct {
	Default             string
	Tools               map[string]string
	PathRules           []PermissionPathRule
	UnsupportedBehavior string
	Metadata            map[string]string
}

type PermissionPathRule struct {
	Path   string
	Action string
}

type Result struct {
	RunID          string
	SessionID      string
	TurnID         string
	Status         string
	TerminalOutput string
	Events         []Event
	EventStats     EventStats
	Memory         MemoryStats
	Artifacts      []Artifact
	Warnings       []string
	Attempts       []AttemptSummary
	Usage          Usage
	EstimatedCost  *CostEstimate
	Policy         PolicySummary
	Permissions    PermissionSummary
	Cleanup        CleanupSummary
	Validation     ValidationSummary
	Repair         RepairSummary
	Error          *Error
	StartedAt      time.Time
	FinishedAt     time.Time
}

type Event struct {
	ID                string
	RunID             string
	SessionID         string
	Time              time.Time
	Type              string
	Kind              string
	Payload           map[string]any
	RawPresent        bool
	RawSafe           bool
	RawOmitted        bool
	RawOmissionReason string
	RawSource         string
	RawEncoding       string
}

type EventStats struct {
	Total    int64
	Retained int
	Dropped  int64
	Limit    int
}

type MemoryStats struct {
	StartAllocBytes uint64
	PeakAllocBytes  uint64
	EndAllocBytes   uint64
	Samples         int64
}

type Artifact struct {
	ID          string
	URI         string
	Kind        string
	Description string
	Metadata    map[string]string
}

const retainedRuntimeEventLimit = 200

type Usage struct {
	InputTokensKnown      bool
	InputTokens           int64
	OutputTokensKnown     bool
	OutputTokens          int64
	TotalTokensKnown      bool
	TotalTokens           int64
	ReasoningTokensKnown  bool
	ReasoningTokens       int64
	CacheReadTokensKnown  bool
	CacheReadTokens       int64
	CacheWriteTokensKnown bool
	CacheWriteTokens      int64
	TurnsKnown            bool
	Turns                 int64
	Native                map[string]any
}

type CostEstimate struct {
	Amount   float64
	Currency string
	Estimate bool
}

type AttemptSummary struct {
	Attempt         int
	AttemptOnTarget int
	TargetIndex     int
	RunID           string
	Status          string
	Provider        string
	Model           string
	ErrorCategory   string
	RateLimited     bool
	RetryAfter      time.Duration
}

type PolicySummary struct {
	FinalAttempt     int
	FinalTargetIndex int
	Exhausted        bool
	ExhaustedReason  string
	Decisions        []PolicyDecision
}

type PolicyDecision struct {
	Attempt     int
	TargetIndex int
	Kind        string
	Reason      string
	Detail      string
	Delay       time.Duration
}

type ValidationSummary struct {
	Configured bool
	Passed     bool
	Failures   int
	Errors     int
	Details    []string
}

type PermissionSummary struct {
	Mode               string
	PolicyID           string
	Default            string
	UnsupportedCount   int
	AuditCount         int
	UnsupportedReasons []string
}

type CleanupSummary struct {
	Attempted bool
	Completed bool
	Failed    bool
	Error     *Error
}

type RepairSummary struct {
	Configured             bool
	Attempted              bool
	MaxAttempts            int
	AttemptCount           int
	Exhausted              bool
	ExhaustedReason        string
	PermissionDenied       bool
	UnsupportedSameSession bool
}

type Error struct {
	Category    string
	Operation   string
	UserDetail  string
	Provider    string
	Model       string
	RuntimeKind string
	ExitCode    *int
	Signal      string
	RetryAfter  time.Duration
	Metadata    map[string]string
}

type Runtime interface {
	StartRun(context.Context, Request) (Result, error)
	Health(context.Context, HealthRequest) (HealthReport, error)
	Capabilities(context.Context) (Capabilities, error)
}

type Adapter struct {
	runtime agentwrap.Runtime
	health  agentwrap.HealthChecker
}

func NewAdapter(aw agentwrap.Runtime) Adapter {
	a := Adapter{runtime: aw}
	if h, ok := aw.(agentwrap.HealthChecker); ok {
		a.health = h
	}
	return a
}

func (a Adapter) StartRun(ctx context.Context, req Request) (Result, error) {
	if a.runtime == nil {
		return Result{}, fmt.Errorf("runtime is required")
	}
	awReq, err := toAgentwrapRequest(req)
	if err != nil {
		return Result{}, err
	}
	run, err := a.runtime.StartRun(ctx, awReq)
	if err != nil {
		return Result{}, mapError(err)
	}

	eventsCh := make(chan eventCollection, 1)
	go func() {
		events := newEventCollection(retainedRuntimeEventLimit)
		for event := range run.Events() {
			events.captureTerminalOutput(event.Payload)
			mapped := mapEvent(event)
			events.add(mapped)
			if req.OnEvent != nil {
				req.OnEvent(mapped)
			}
		}
		events.finish()
		eventsCh <- events
	}()

	type waitResult struct {
		result agentwrap.RunResult
		err    error
	}
	waitCh := make(chan waitResult, 1)
	go func() {
		result, err := run.Wait(ctx)
		waitCh <- waitResult{result: result, err: err}
	}()

	var result agentwrap.RunResult
	var waitErr error
	select {
	case waited := <-waitCh:
		result = waited.result
		waitErr = waited.err
	case <-ctx.Done():
		_ = run.Cancel(context.Background())
		select {
		case waited := <-waitCh:
			result = waited.result
			waitErr = waited.err
		case <-time.After(5 * time.Second):
			mapped := Result{
				Status:     "cancelled",
				FinishedAt: time.Now(),
				Error:      &Error{Category: "cancellation", Operation: "run", UserDetail: ctx.Err().Error()},
			}
			return mapped, ctx.Err()
		}
		if waitErr == nil {
			waitErr = ctx.Err()
		}
	}

	mapped := mapResult(result)
	select {
	case events := <-eventsCh:
		mapped.Events = events.events
		if mapped.TerminalOutput == "" && events.terminalOutput != "" {
			mapped.TerminalOutput = events.terminalOutput
		}
		mapped.EventStats = events.stats()
		mapped.Memory = events.memory
		if events.dropped > 0 {
			mapped.Warnings = append(mapped.Warnings, fmt.Sprintf("runtime retained last %d events and dropped %d earlier events from in-memory result", events.limit, events.dropped))
		}
	case <-time.After(time.Second):
	}
	if waitErr != nil {
		if errors.Is(waitErr, context.Canceled) {
			mapped.Status = "cancelled"
			mapped.Error = &Error{Category: "cancellation", Operation: "run", UserDetail: waitErr.Error()}
		} else if errors.Is(waitErr, context.DeadlineExceeded) {
			mapped.Status = "failed"
			mapped.Error = &Error{Category: "timeout", Operation: "run", UserDetail: waitErr.Error()}
		}
		return mapped, mapError(waitErr)
	}
	if result.Err != nil {
		return mapped, mapError(result.Err)
	}
	return mapped, nil
}

type eventCollection struct {
	events         []Event
	terminalOutput string
	total          int64
	dropped        int64
	limit          int
	memory         MemoryStats
}

func newEventCollection(limit int) eventCollection {
	if limit < 0 {
		limit = 0
	}
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)
	return eventCollection{
		events: make([]Event, 0, limit),
		limit:  limit,
		memory: MemoryStats{
			StartAllocBytes: mem.Alloc,
			PeakAllocBytes:  mem.Alloc,
			Samples:         1,
		},
	}
}

func (c *eventCollection) add(event Event) {
	c.total++
	if c.total == 1 || c.total%64 == 0 {
		c.sampleMemory()
	}
	if c.limit == 0 {
		c.dropped++
		return
	}
	if len(c.events) < c.limit {
		c.events = append(c.events, event)
		return
	}
	copy(c.events, c.events[1:])
	c.events[len(c.events)-1] = event
	c.dropped++
}

func (c *eventCollection) captureTerminalOutput(payload map[string]any) {
	if value := terminalOutputValue(payload, 0); value != "" {
		c.terminalOutput = truncateString(value, maxMappedTerminalOutputBytes)
	}
}

func terminalOutputValue(value any, depth int) string {
	if depth > 4 {
		return ""
	}
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"structured_output", "output", "content", "text", "message", "part"} {
			if nested, ok := typed[key]; ok {
				if result := terminalOutputValue(nested, depth+1); result != "" {
					return result
				}
			}
		}
	case []any:
		for i := len(typed) - 1; i >= 0; i-- {
			if result := terminalOutputValue(typed[i], depth+1); result != "" {
				return result
			}
		}
	case string:
		return typed
	}
	return ""
}

func (c *eventCollection) finish() {
	c.sampleMemory()
}

func (c *eventCollection) sampleMemory() {
	var mem stdruntime.MemStats
	stdruntime.ReadMemStats(&mem)
	c.memory.Samples++
	c.memory.EndAllocBytes = mem.Alloc
	if mem.Alloc > c.memory.PeakAllocBytes {
		c.memory.PeakAllocBytes = mem.Alloc
	}
}

func (c eventCollection) stats() EventStats {
	return EventStats{
		Total:    c.total,
		Retained: len(c.events),
		Dropped:  c.dropped,
		Limit:    c.limit,
	}
}

func (a Adapter) Capabilities(ctx context.Context) (Capabilities, error) {
	if a.runtime == nil {
		return Capabilities{}, fmt.Errorf("runtime is required")
	}
	caps, err := a.runtime.Capabilities(ctx)
	if err != nil {
		return Capabilities{}, mapError(err)
	}
	return mapCapabilities(caps), nil
}

func RequestFromConfig(c config.Config, workDir string) (Request, error) {
	timeout, err := time.ParseDuration(c.Execution.DefaultTimeout)
	if err != nil {
		return Request{}, err
	}
	provider, model := splitModel(c.Models.Primary)
	if provider == "" && model == "" {
		provider, model = splitModel(c.Models.Default)
	}
	return Request{
		WorkDir:       workDir,
		Provider:      provider,
		Model:         model,
		Timeout:       timeout,
		RequireHealth: append([]string(nil), c.Agentwrap.RequiredHealth...),
		RequireCaps:   append([]string(nil), c.Agentwrap.RequiredCapabilities...),
		Sandbox:       c.Agentwrap.Sandbox,
		Permissions:   c.Agentwrap.PermissionMode,
		Policy: PermissionPolicy{
			Default:             c.Agentwrap.PermissionDefault,
			UnsupportedBehavior: c.Agentwrap.PermissionUnsupportedBehavior,
		},
	}, nil
}

func toAgentwrapRequest(req Request) (agentwrap.RunRequest, error) {
	health, err := mapHealthIDs(req.RequireHealth)
	if err != nil {
		return agentwrap.RunRequest{}, err
	}
	caps, err := mapCapabilitiesIDs(req.RequireCaps)
	if err != nil {
		return agentwrap.RunRequest{}, err
	}
	policy, err := mapPermissionPolicy(req.Policy)
	if err != nil {
		return agentwrap.RunRequest{}, err
	}
	metadata := cloneStringMap(req.Metadata)
	if req.Cache.Key != "" {
		if metadata == nil {
			metadata = map[string]string{}
		}
		cohort, _ := json.Marshal(struct {
			Foundation, Provider, Model, WorkDir, Sandbox, Permissions string
			Policy                                                     PermissionPolicy
		}{req.Cache.Key, req.Provider, req.Model, req.WorkDir, req.Sandbox, req.Permissions, req.Policy})
		sum := sha256.Sum256(cohort)
		metadata["prompt_cache_foundation_key"] = req.Cache.Key
		metadata["prompt_cache_key"] = "ultraplan-cohort-v1-" + hex.EncodeToString(sum[:16])
		metadata["prompt_cache_breakpoint_bytes"] = fmt.Sprintf("%d", req.Cache.BreakpointBytes)
		metadata["prompt_cache_prefix_sha256"] = req.Cache.PrefixDigest
		metadata["prompt_cache_mode"] = req.Cache.Mode
	}
	return agentwrap.RunRequest{
		Prompt:           req.Prompt,
		WorkDir:          req.WorkDir,
		SessionID:        agentwrap.SessionID(req.SessionID),
		SessionAction:    agentwrap.SessionAction(req.SessionAction),
		Provider:         agentwrap.ProviderID(req.Provider),
		Model:            agentwrap.ModelID(req.Model),
		Timeout:          req.Timeout,
		Metadata:         metadata,
		RequireHealth:    health,
		RequireCaps:      caps,
		Sandbox:          agentwrap.SandboxMode(req.Sandbox),
		Permissions:      agentwrap.PermissionMode(req.Permissions),
		PermissionPolicy: policy,
		Validation:       req.Validation,
	}, nil
}

func mapResult(result agentwrap.RunResult) Result {
	warnings := make([]string, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, truncateDiagnosticString(warning))
	}
	return Result{
		RunID:          string(result.RunID),
		SessionID:      string(result.SessionID),
		TurnID:         string(result.TurnID),
		Status:         string(result.Status),
		TerminalOutput: result.TerminalOutput,
		Artifacts:      mapArtifacts(result.Artifacts),
		Warnings:       warnings,
		Attempts:       mapAttempts(result.Metadata.Attempts),
		Usage:          mapUsage(result.Usage),
		EstimatedCost:  mapCost(result.Metadata.EstimatedCost),
		Policy:         mapPolicy(result.Metadata.Policy),
		Permissions:    mapPermissions(result.Metadata.Permissions),
		Cleanup:        mapCleanup(result.Metadata.Cleanup),
		Validation:     mapValidation(result.Metadata.Validation),
		Repair:         mapRepair(result.Metadata.Repair),
		Error:          mapSDKError(result.Err),
		StartedAt:      result.StartedAt,
		FinishedAt:     result.FinishedAt,
	}
}

func mapEvent(event agentwrap.Event) Event {
	rawPresent := event.Raw != nil
	rawSafe := rawPresent && event.Raw.Safe
	return Event{
		ID:                string(event.ID),
		RunID:             string(event.RunID),
		SessionID:         string(event.SessionID),
		Time:              event.Time,
		Type:              event.Type,
		Kind:              string(event.Kind()),
		Payload:           cloneAnyMap(event.Payload),
		RawPresent:        rawPresent,
		RawSafe:           rawSafe,
		RawOmitted:        rawPresent,
		RawOmissionReason: rawOmissionReason(rawPresent, rawSafe),
		RawSource:         rawSource(event.Raw),
		RawEncoding:       rawEncoding(event.Raw),
	}
}

func mapUsage(usage agentwrap.Usage) Usage {
	out := Usage{Native: cloneAnyMap(usage.Native)}
	if usage.InputTokens != nil {
		out.InputTokensKnown = true
		out.InputTokens = *usage.InputTokens
	}
	if usage.OutputTokens != nil {
		out.OutputTokensKnown = true
		out.OutputTokens = *usage.OutputTokens
	}
	if usage.TotalTokens != nil {
		out.TotalTokensKnown = true
		out.TotalTokens = *usage.TotalTokens
	}
	if usage.ReasoningTokens != nil {
		out.ReasoningTokensKnown = true
		out.ReasoningTokens = *usage.ReasoningTokens
	}
	if usage.CacheReadTokens != nil {
		out.CacheReadTokensKnown = true
		out.CacheReadTokens = *usage.CacheReadTokens
	}
	if usage.CacheWriteTokens != nil {
		out.CacheWriteTokensKnown = true
		out.CacheWriteTokens = *usage.CacheWriteTokens
	}
	if usage.Turns != nil {
		out.TurnsKnown = true
		out.Turns = *usage.Turns
	}
	return out
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	var sdk *agentwrap.SDKError
	if errors.As(err, &sdk) {
		bounded := *sdk
		bounded.UserDetail = redactDiagnosticString("sdk.user_detail", sdk.UserDetail)
		bounded.DebugDetail = redactDiagnosticString("sdk.debug_detail", sdk.DebugDetail)
		bounded.ResponseBody = redactDiagnosticString("sdk.response_body", sdk.ResponseBody)
		bounded.ResponseHeaders = cloneStringMap(sdk.ResponseHeaders)
		bounded.Metadata = cloneStringMap(sdk.Metadata)
		switch {
		case errors.Is(err, context.Canceled):
			bounded.Cause = context.Canceled
		case errors.Is(err, context.DeadlineExceeded):
			bounded.Cause = context.DeadlineExceeded
		default:
			bounded.Cause = nil
		}
		return &bounded
	}
	return err
}

func mapSDKError(err *agentwrap.SDKError) *Error {
	if err == nil {
		return nil
	}
	return &Error{
		Category:    string(err.Category),
		Operation:   err.Operation,
		UserDetail:  redactDiagnosticString("sdk.user_detail", err.UserDetail),
		Provider:    string(err.Provider),
		Model:       string(err.Model),
		RuntimeKind: string(err.RuntimeKind),
		ExitCode:    err.ExitCode,
		Signal:      err.Signal,
		RetryAfter:  err.RetryAfter,
		Metadata:    cloneStringMap(err.Metadata),
	}
}

func redactDiagnosticString(key, value string) string {
	return config.RedactValue(key, truncateDiagnosticString(value))
}

func splitModel(value string) (string, string) {
	for i, r := range value {
		if r == '/' {
			return value[:i], value[i+1:]
		}
	}
	return "", value
}
