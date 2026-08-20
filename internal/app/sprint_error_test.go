package app

import (
	"context"
	"errors"
	"testing"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func TestMapSmokeErrorPreservesTypedCause(t *testing.T) {
	source := &sprint.SmokeError{
		Code:     "smoke_timeout",
		Category: "timeout",
		Message:  "harness timed out",
		Guidance: "retry with a bounded timeout",
		Err:      errors.New("deadline exceeded"),
	}
	mapped := mapSmokeError(source)
	if !errors.Is(mapped, source) {
		t.Fatalf("mapped error lost typed source: %#v", mapped)
	}
	got, ok := sprint.AsSmokeError(mapped)
	if !ok || got != source {
		t.Fatalf("mapped error cannot be recovered with errors.As: %#v", mapped)
	}
}

func TestFailedOperationDoesNotClassifyFromErrorText(t *testing.T) {
	source := errors.New("runtime validation lock provider missing")
	result, returned := failedOperation(OperationResult{}, source)
	if !errors.Is(returned, source) {
		t.Fatalf("returned error lost cause: %v", returned)
	}
	if result.Error == nil || result.Error.Code != "internal.error" || result.Error.Category != "internal" {
		t.Fatalf("free-form text affected classification: %+v", result.Error)
	}
	if result.Message != "Operation failed." || result.Error.Cause != source.Error() || result.Error.Cause == result.Error.Message {
		t.Fatalf("message/cause separation failed: result=%+v error=%+v", result, result.Error)
	}
}

func TestFailedOperationUsesTypedCancellation(t *testing.T) {
	result, _ := failedOperation(OperationResult{}, context.Canceled)
	if result.Error == nil || result.Error.Code != "workflow.cancelled" || result.State != OperationCancelled {
		t.Fatalf("typed cancellation classification failed: %+v", result)
	}
}
