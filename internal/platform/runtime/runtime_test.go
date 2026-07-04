package runtime

import (
	"context"
	"errors"
	"strings"
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

func TestAdapterRetainsOnlyRecentRuntimeEvents(t *testing.T) {
	events := make([]agentwrap.Event, retainedRuntimeEventLimit+5)
	for i := range events {
		events[i] = agentwrap.Event{
			ID:      agentwrap.EventID("event-" + string(rune('a'+i%26))),
			RunID:   "run-1",
			Type:    "native.message",
			Payload: agentwrap.EventPayloadWithKind(agentwrap.EventMessage, agentwrap.EventPayload{"index": i}),
		}
	}
	adapter := NewAdapter(fakeRuntime{run: &fakeRun{
		id:     "run-1",
		events: events,
		result: agentwrap.RunResult{RunID: "run-1", Status: agentwrap.StatusCompleted},
	}})

	result, err := adapter.StartRun(context.Background(), Request{Prompt: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Events) != retainedRuntimeEventLimit {
		t.Fatalf("retained events = %d, want %d", len(result.Events), retainedRuntimeEventLimit)
	}
	if result.EventStats.Total != int64(len(events)) || result.EventStats.Dropped != 5 || result.EventStats.Retained != retainedRuntimeEventLimit {
		t.Fatalf("unexpected event stats: %+v", result.EventStats)
	}
	if result.Events[0].Payload["index"] != 5 {
		t.Fatalf("oldest retained event = %+v", result.Events[0])
	}
	if result.Memory.Samples == 0 || result.Memory.PeakAllocBytes == 0 {
		t.Fatalf("memory stats not sampled: %+v", result.Memory)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected truncation warning: %+v", result.Warnings)
	}
}

func TestAdapterBoundsMappedEventPayloads(t *testing.T) {
	huge := strings.Repeat("x", maxMappedPayloadStringBytes+512)
	var forwarded Event
	adapter := NewAdapter(fakeRuntime{run: &fakeRun{
		id: "run-1",
		events: []agentwrap.Event{{
			ID:    "event-1",
			RunID: "run-1",
			Type:  "native.warning",
			Payload: agentwrap.EventPayloadWithKind(agentwrap.EventWarning, agentwrap.EventPayload{
				"detail": huge,
				"raw":    []byte(huge),
				"nested": map[string]any{"message": huge},
			}),
		}},
		result: agentwrap.RunResult{
			RunID:  "run-1",
			Status: agentwrap.StatusCompleted,
			Usage:  agentwrap.Usage{Native: map[string]any{"payload": huge}},
		},
	}})

	result, err := adapter.StartRun(context.Background(), Request{
		Prompt: "hello",
		OnEvent: func(event Event) {
			forwarded = event
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range []map[string]any{forwarded.Payload, result.Events[0].Payload, result.Usage.Native} {
		value, ok := payload["detail"].(string)
		if !ok {
			value, ok = payload["payload"].(string)
		}
		if !ok || len(value) > maxMappedPayloadStringBytes+64 || !strings.Contains(value, "truncated") {
			t.Fatalf("payload string not bounded: len=%d value=%q payload=%+v", len(value), value, payload)
		}
	}
	if raw, ok := result.Events[0].Payload["raw"].(string); !ok || !strings.Contains(raw, "bytes omitted") {
		t.Fatalf("raw bytes not omitted: %+v", result.Events[0].Payload["raw"])
	}
}

func TestAdapterMapsPolicyRetryFallbackAndValidationMetadata(t *testing.T) {
	total := int64(99)
	adapter := NewAdapter(fakeRuntime{run: &fakeRun{
		id: "run-1",
		result: agentwrap.RunResult{
			RunID:  "run-1",
			Status: agentwrap.StatusFailed,
			Usage:  agentwrap.Usage{TotalTokens: &total},
			Metadata: agentwrap.RunMetadata{
				Attempts: []agentwrap.AttemptSummary{{
					Attempt:         1,
					AttemptOnTarget: 1,
					TargetIndex:     0,
					RunID:           "attempt-1",
					Status:          agentwrap.StatusFailed,
					Context:         agentwrap.RuntimeContext{Provider: "anthropic", Model: "claude"},
					ErrorCategory:   agentwrap.ErrorRateLimit,
					RateLimit:       &agentwrap.RateLimitInfo{RetryAfter: time.Minute},
				}},
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
				Permissions: agentwrap.PermissionMetadata{
					Mode:     "restricted",
					PolicyID: "policy-1",
					Policy:   agentwrap.PermissionPolicySummary{Default: agentwrap.PermissionActionAsk},
					Unsupported: []agentwrap.PermissionFeatureSupport{{
						Feature: "path:/tmp/outside",
						Reason:  "unsupported path rule",
					}},
				},
				Cleanup: agentwrap.CleanupMetadata{Attempted: true, Completed: true},
				Validation: agentwrap.ValidationMetadata{
					Configured: true,
					Final: agentwrap.ValidationResult{
						Passed:      false,
						FailedCount: 1,
						Errors:      []agentwrap.SDKError{*agentwrap.NewError(agentwrap.ErrorValidation, "validation", "failed", nil)},
					},
				},
				Repair:        agentwrap.RepairMetadata{Configured: true, Attempted: true, MaxAttempts: 2, Attempts: []agentwrap.RepairAttemptSummary{{Attempt: 1}}, Exhausted: true, PermissionDenied: true},
				EstimatedCost: &agentwrap.CostEstimate{Amount: 0.25, Currency: "USD", Estimate: true},
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
	if len(result.Attempts) != 1 || !result.Attempts[0].RateLimited || result.Attempts[0].RetryAfter != time.Minute || result.Attempts[0].Provider != "anthropic" {
		t.Fatalf("attempt metadata not mapped: %+v", result.Attempts)
	}
	if result.Permissions.PolicyID != "policy-1" || result.Permissions.Default != "ask" || result.Permissions.UnsupportedCount != 1 {
		t.Fatalf("permission metadata not mapped: %+v", result.Permissions)
	}
	if !result.Cleanup.Attempted || !result.Cleanup.Completed {
		t.Fatalf("cleanup metadata not mapped: %+v", result.Cleanup)
	}
	if !result.Repair.Attempted || result.Repair.AttemptCount != 1 || !result.Repair.PermissionDenied {
		t.Fatalf("repair metadata not mapped: %+v", result.Repair)
	}
	if !result.Usage.TotalTokensKnown || result.Usage.TotalTokens != 99 || result.EstimatedCost == nil || result.EstimatedCost.Amount != 0.25 {
		t.Fatalf("usage/cost metadata not mapped: usage=%+v cost=%+v", result.Usage, result.EstimatedCost)
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

func TestStartRunCancelsUnderlyingRunWhenContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	run := &fakeRun{id: "run-cancel", waitForCancel: true}
	adapter := NewAdapter(fakeRuntime{run: run})
	cancel()

	result, err := adapter.StartRun(ctx, Request{Prompt: "hello"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if !run.cancelled {
		t.Fatal("underlying run was not cancelled")
	}
	if result.Status != "cancelled" || result.Error == nil || result.Error.Category != "cancellation" {
		t.Fatalf("result = %+v", result)
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
	id            agentwrap.RunID
	events        []agentwrap.Event
	result        agentwrap.RunResult
	err           error
	waitForCancel bool
	cancelled     bool
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

func (f *fakeRun) Wait(ctx context.Context) (agentwrap.RunResult, error) {
	if f.waitForCancel {
		<-ctx.Done()
		return agentwrap.RunResult{Status: agentwrap.StatusCancelled}, ctx.Err()
	}
	return f.result, f.err
}

func (f *fakeRun) Cancel(context.Context) error {
	f.cancelled = true
	return nil
}
