// Package runtime provides UltraPlan's generic agent runtime boundary.
package runtime

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Antonio7098/agentwrap"

	"ultraplan-go/internal/platform/config"
)

type Request struct {
	Prompt        string
	WorkDir       string
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
	RunID         string
	SessionID     string
	TurnID        string
	Status        string
	Events        []Event
	Artifacts     []Artifact
	Warnings      []string
	Usage         Usage
	EstimatedCost *CostEstimate
	Policy        PolicySummary
	Validation    ValidationSummary
	Error         *Error
	StartedAt     time.Time
	FinishedAt    time.Time
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

type Artifact struct {
	ID          string
	URI         string
	Kind        string
	Description string
	Metadata    map[string]string
}

type Usage struct {
	InputTokensKnown  bool
	InputTokens       int64
	OutputTokensKnown bool
	OutputTokens      int64
	TotalTokensKnown  bool
	TotalTokens       int64
	Native            map[string]any
}

type CostEstimate struct {
	Amount   float64
	Currency string
	Estimate bool
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

	eventsCh := make(chan []Event, 1)
	go func() {
		events := []Event{}
		for event := range run.Events() {
			events = append(events, mapEvent(event))
		}
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
		mapped.Events = events
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
	return agentwrap.RunRequest{
		Prompt:           req.Prompt,
		WorkDir:          req.WorkDir,
		Provider:         agentwrap.ProviderID(req.Provider),
		Model:            agentwrap.ModelID(req.Model),
		Timeout:          req.Timeout,
		Metadata:         cloneStringMap(req.Metadata),
		RequireHealth:    health,
		RequireCaps:      caps,
		Sandbox:          agentwrap.SandboxMode(req.Sandbox),
		Permissions:      agentwrap.PermissionMode(req.Permissions),
		PermissionPolicy: policy,
		Validation:       req.Validation,
	}, nil
}

func mapResult(result agentwrap.RunResult) Result {
	return Result{
		RunID:         string(result.RunID),
		SessionID:     string(result.SessionID),
		TurnID:        string(result.TurnID),
		Status:        string(result.Status),
		Artifacts:     mapArtifacts(result.Artifacts),
		Warnings:      append([]string(nil), result.Warnings...),
		Usage:         mapUsage(result.Usage),
		EstimatedCost: mapCost(result.Metadata.EstimatedCost),
		Policy:        mapPolicy(result.Metadata.Policy),
		Validation:    mapValidation(result.Metadata.Validation),
		Error:         mapSDKError(result.Err),
		StartedAt:     result.StartedAt,
		FinishedAt:    result.FinishedAt,
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
	return out
}

func mapError(err error) error {
	if err == nil {
		return nil
	}
	var sdk *agentwrap.SDKError
	if errors.As(err, &sdk) {
		return sdk
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
		UserDetail:  err.UserDetail,
		Provider:    string(err.Provider),
		Model:       string(err.Model),
		RuntimeKind: string(err.RuntimeKind),
		ExitCode:    err.ExitCode,
		Signal:      err.Signal,
		RetryAfter:  err.RetryAfter,
		Metadata:    cloneStringMap(err.Metadata),
	}
}

func splitModel(value string) (string, string) {
	for i, r := range value {
		if r == '/' {
			return value[:i], value[i+1:]
		}
	}
	return "", value
}
