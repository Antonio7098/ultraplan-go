package app

import (
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
