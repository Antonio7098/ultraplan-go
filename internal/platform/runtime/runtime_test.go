package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Antonio7098/agentwrap"
)

func TestAdapterStartRunMapsEventsUsageAndError(t *testing.T) {
	input := int64(12)
	run := &fakeRun{
		id: "run-1",
		events: []agentwrap.Event{{
			ID:      "event-1",
			RunID:   "run-1",
			Type:    "native.message",
			Payload: agentwrap.EventPayloadWithKind(agentwrap.EventMessage, agentwrap.EventPayload{"text": "ok"}),
			Raw:     &agentwrap.RawPayload{Source: "stdout", Encoding: "json", Data: []byte(`{"secret":"x"}`), Safe: false},
		}},
		result: agentwrap.RunResult{
			RunID:  "run-1",
			Status: agentwrap.StatusCompleted,
			Usage:  agentwrap.Usage{InputTokens: &input},
			Metadata: agentwrap.RunMetadata{
				Policy: agentwrap.PolicyMetadata{FinalAttempt: 1},
			},
		},
	}
	adapter := NewAdapter(fakeRuntime{run: run})

	result, err := adapter.StartRun(context.Background(), Request{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if result.RunID != "run-1" || result.Status != "completed" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Events) != 1 || result.Events[0].Kind != "message" || !result.Events[0].RawOmitted {
		t.Fatalf("event not mapped safely: %+v", result.Events)
	}
	if !result.Usage.InputTokensKnown || result.Usage.InputTokens != 12 || result.Usage.OutputTokensKnown {
		t.Fatalf("usage knownness not preserved: %+v", result.Usage)
	}
}

func TestAdapterPreservesSDKErrorClassification(t *testing.T) {
	sdkErr := agentwrap.NewError(agentwrap.ErrorRateLimit, "fake wait", "slow down", errors.New("429"), agentwrap.WithRetryAfter(time.Second))
	adapter := NewAdapter(fakeRuntime{run: &fakeRun{id: "run-1", result: agentwrap.RunResult{RunID: "run-1", Status: agentwrap.StatusFailed, Err: sdkErr}, err: sdkErr}})

	result, err := adapter.StartRun(context.Background(), Request{Prompt: "hello"})
	if err == nil {
		t.Fatal("expected error")
	}
	var got *agentwrap.SDKError
	if !errors.As(err, &got) || got.Category != agentwrap.ErrorRateLimit || got.RetryAfter != time.Second {
		t.Fatalf("classification not preserved: %#v", err)
	}
	if result.Error == nil || result.Error.Category != "rate_limit" {
		t.Fatalf("mapped result error missing: %+v", result.Error)
	}
}

func TestAdapterMapsAllCanonicalEventKinds(t *testing.T) {
	kinds := []agentwrap.EventKind{
		agentwrap.EventLifecycle,
		agentwrap.EventSession,
		agentwrap.EventMessage,
		agentwrap.EventProgress,
		agentwrap.EventTool,
		agentwrap.EventArtifact,
		agentwrap.EventPermission,
		agentwrap.EventBlocking,
		agentwrap.EventUsage,
		agentwrap.EventWarning,
		agentwrap.EventFatalError,
		agentwrap.EventRateLimit,
		agentwrap.EventValidation,
		agentwrap.EventRetry,
		agentwrap.EventFallback,
		agentwrap.EventFinalResult,
		agentwrap.EventNativeExtension,
	}
	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			event := agentwrap.Event{
				ID:      agentwrap.EventID("event-" + kind),
				RunID:   "run-1",
				Type:    "native." + string(kind),
				Payload: agentwrap.EventPayloadWithKind(kind, agentwrap.EventPayload{"value": string(kind)}),
			}

			got := mapEvent(event)
			if got.Kind != string(kind) {
				t.Fatalf("kind = %q, want %q", got.Kind, kind)
			}
			if got.Type != event.Type || got.Payload["value"] != string(kind) {
				t.Fatalf("event details not preserved: %+v", got)
			}
		})
	}
}

func TestAdapterMapsCancellationAndTimeoutFailures(t *testing.T) {
	tests := []struct {
		name     string
		category agentwrap.ErrorCategory
		status   agentwrap.RunStatus
		cause    error
	}{
		{name: "cancellation", category: agentwrap.ErrorCancellation, status: agentwrap.StatusCancelled, cause: context.Canceled},
		{name: "timeout", category: agentwrap.ErrorTimeout, status: agentwrap.StatusFailed, cause: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sdkErr := agentwrap.NewError(tt.category, "fake wait", string(tt.category), tt.cause)
			adapter := NewAdapter(fakeRuntime{run: &fakeRun{
				id:     "run-1",
				result: agentwrap.RunResult{RunID: "run-1", Status: tt.status, Err: sdkErr},
				err:    sdkErr,
			}})

			result, err := adapter.StartRun(context.Background(), Request{Prompt: "hello"})
			if err == nil {
				t.Fatal("expected error")
			}
			var got *agentwrap.SDKError
			if !errors.As(err, &got) || got.Category != tt.category {
				t.Fatalf("classification not preserved: %#v", err)
			}
			if result.Status != string(tt.status) || result.Error == nil || result.Error.Category != string(tt.category) {
				t.Fatalf("result error not mapped: %+v", result)
			}
		})
	}
}

func TestAdapterMapsMalformedEventFailureSafely(t *testing.T) {
	sdkErr := agentwrap.NewError(agentwrap.ErrorMalformedEvent, "fake projector", "malformed event", errors.New("bad json"))
	adapter := NewAdapter(fakeRuntime{run: &fakeRun{
		id: "run-1",
		events: []agentwrap.Event{{
			ID:      "event-1",
			RunID:   "run-1",
			Type:    "native.bad",
			Payload: agentwrap.EventPayloadWithKind(agentwrap.EventNativeExtension, agentwrap.EventPayload{"malformed": true}),
			Raw:     &agentwrap.RawPayload{Source: "stdout", Encoding: "json", Safe: false},
		}},
		result: agentwrap.RunResult{RunID: "run-1", Status: agentwrap.StatusFailed, Err: sdkErr},
		err:    sdkErr,
	}})

	result, err := adapter.StartRun(context.Background(), Request{Prompt: "hello"})
	if err == nil {
		t.Fatal("expected malformed event error")
	}
	var got *agentwrap.SDKError
	if !errors.As(err, &got) || got.Category != agentwrap.ErrorMalformedEvent {
		t.Fatalf("classification not preserved: %#v", err)
	}
	if len(result.Events) != 1 || result.Events[0].Kind != "native_extension" || !result.Events[0].RawOmitted {
		t.Fatalf("malformed event facts not preserved safely: %+v", result.Events)
	}
}

func TestAdapterMapsPolicyRetryFallbackAndValidationMetadata(t *testing.T) {
	adapter := NewAdapter(fakeRuntime{run: &fakeRun{
		id: "run-1",
		result: agentwrap.RunResult{
			RunID:  "run-1",
			Status: agentwrap.StatusFailed,
			Metadata: agentwrap.RunMetadata{
				Policy: agentwrap.PolicyMetadata{
					FinalAttempt:     2,
					FinalTargetIndex: 1,
					Exhausted:        true,
					ExhaustedReason:  "retry exhausted",
					Decisions: []agentwrap.PolicyDecisionRecord{
						{Attempt: 1, TargetIndex: 0, Kind: agentwrap.PolicyDecisionRetry, Reason: "rate limit", Delay: time.Second},
						{Attempt: 2, TargetIndex: 0, Kind: agentwrap.PolicyDecisionFallback, Reason: "runtime exit"},
					},
				},
				Validation: agentwrap.ValidationMetadata{
					Configured: true,
					Final: agentwrap.ValidationResult{
						Passed:      false,
						FailedCount: 1,
						Errors:      []agentwrap.SDKError{*agentwrap.NewError(agentwrap.ErrorValidation, "validation", "failed", nil)},
					},
				},
			},
		},
	}})

	result, err := adapter.StartRun(context.Background(), Request{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Policy.Exhausted || len(result.Policy.Decisions) != 2 || result.Policy.Decisions[0].Kind != "retry" || result.Policy.Decisions[1].Kind != "fallback" {
		t.Fatalf("policy metadata not mapped: %+v", result.Policy)
	}
	if !result.Validation.Configured || result.Validation.Passed || result.Validation.Failures != 1 || result.Validation.Errors != 1 {
		t.Fatalf("validation metadata not mapped: %+v", result.Validation)
	}
}

func TestPermissionPathRulesFailUnlessBestEffort(t *testing.T) {
	_, err := mapPermissionPolicy(PermissionPolicy{
		Default:   "ask",
		PathRules: []PermissionPathRule{{Path: "secret", Action: "deny"}},
	})
	if err == nil {
		t.Fatal("expected unsupported path rule error")
	}
	policy, err := mapPermissionPolicy(PermissionPolicy{
		Default:             "ask",
		UnsupportedBehavior: "best_effort",
		PathRules:           []PermissionPathRule{{Path: "secret", Action: "deny"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if policy.UnsupportedBehavior != agentwrap.PermissionUnsupportedBestEffort {
		t.Fatalf("unexpected policy: %+v", policy)
	}
}

func TestHealthAndCapabilityNameValidation(t *testing.T) {
	if _, err := mapHealthIDs([]string{"runtime_available", "authentication"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mapCapabilitiesIDs([]string{"structured_events", "validation_events"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mapHealthIDs([]string{"unknown"}); err == nil {
		t.Fatal("expected unknown health error")
	}
	if _, err := mapCapabilitiesIDs([]string{"unknown"}); err == nil {
		t.Fatal("expected unknown capability error")
	}
}

type fakeRuntime struct {
	run *fakeRun
}

func (f fakeRuntime) StartRun(context.Context, agentwrap.RunRequest) (agentwrap.Run, error) {
	return f.run, nil
}

func (f fakeRuntime) Capabilities(context.Context) (agentwrap.Capabilities, error) {
	return agentwrap.Capabilities{RuntimeKind: "fake", Features: map[agentwrap.Capability]agentwrap.CapabilitySupport{
		agentwrap.CapabilityStructuredEvents: {Supported: true, Detail: "fake events"},
	}}, nil
}

type fakeRun struct {
	id     agentwrap.RunID
	events []agentwrap.Event
	result agentwrap.RunResult
	err    error
}

func (f *fakeRun) ID() agentwrap.RunID { return f.id }

func (f *fakeRun) Events() <-chan agentwrap.Event {
	ch := make(chan agentwrap.Event, len(f.events))
	for _, event := range f.events {
		ch <- event
	}
	close(ch)
	return ch
}

func (f *fakeRun) Wait(context.Context) (agentwrap.RunResult, error) {
	return f.result, f.err
}

func (f *fakeRun) Cancel(context.Context) error { return nil }
