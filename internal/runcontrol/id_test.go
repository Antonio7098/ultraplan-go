package runcontrol

import (
	"errors"
	"strings"
	"testing"
)

func TestRandomIDSourceProducesUniqueCanonicalOpaqueIDs(t *testing.T) {
	t.Parallel()
	source := RandomIDSource{}
	seen := make(map[string]struct{}, 4000)
	for i := 0; i < 2000; i++ {
		runID, err := source.NewRunID()
		if err != nil {
			t.Fatalf("NewRunID: %v", err)
		}
		attemptID, err := source.NewAttemptID()
		if err != nil {
			t.Fatalf("NewAttemptID: %v", err)
		}
		if err := runID.Validate(); err != nil {
			t.Fatalf("generated run ID %q is invalid: %v", runID, err)
		}
		if err := attemptID.Validate(); err != nil {
			t.Fatalf("generated attempt ID %q is invalid: %v", attemptID, err)
		}
		for _, value := range []string{string(runID), string(attemptID)} {
			if value != strings.ToLower(value) {
				t.Fatalf("generated ID is not canonical lowercase: %q", value)
			}
			if _, exists := seen[value]; exists {
				t.Fatalf("duplicate generated identity %q", value)
			}
			seen[value] = struct{}{}
		}
	}
}

func TestOpaqueIDValidationRejectsEncodedOrNonCanonicalValues(t *testing.T) {
	t.Parallel()
	invalid := []RunID{
		"",
		"run_abc",
		"op_aaaaaaaaaaaaaaaaaaaaaaaaaa",
		"run_AAAAAAAAAAAAAAAAAAAAAAAAAA",
		"run_00000000000000000000000000",
		"run_aaaaaaaaaaaaaaaaaaaaaaaaab",
	}
	for _, id := range invalid {
		if err := id.Validate(); !errors.Is(err, ErrInvalidArgument) {
			t.Errorf("RunID(%q).Validate() error = %v, want invalid argument", id, err)
		}
	}
}

func TestRandomIDSourceReportsEntropyFailure(t *testing.T) {
	t.Parallel()
	source := RandomIDSource{Reader: failingReader{}}
	if _, err := source.NewRunID(); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewRunID error = %v, want unavailable", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("entropy unavailable") }
