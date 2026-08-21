package runcontrol

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"strings"
)

const (
	randomIDBytes  = 16
	encodedIDBytes = 26
)

var idEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// RunID is an opaque workspace-scoped identity. Its encoded value carries no
// time, path, process, provider, or ownership information.
type RunID string

// AttemptID identifies one fenced ownership attempt within a run.
type AttemptID string

// IDSource generates opaque run and attempt identities.
type IDSource interface {
	NewRunID() (RunID, error)
	NewAttemptID() (AttemptID, error)
}

// RandomIDSource reads 128 bits from Reader for every generated identity.
// Reader defaults to crypto/rand.Reader.
type RandomIDSource struct {
	Reader io.Reader
}

func (s RandomIDSource) NewRunID() (RunID, error) {
	id, err := s.generate("run_")
	if err != nil {
		return "", err
	}
	return RunID(id), nil
}

func (s RandomIDSource) NewAttemptID() (AttemptID, error) {
	id, err := s.generate("att_")
	if err != nil {
		return "", err
	}
	return AttemptID(id), nil
}

func (s RandomIDSource) generate(prefix string) (string, error) {
	reader := s.Reader
	if reader == nil {
		reader = rand.Reader
	}
	raw := make([]byte, randomIDBytes)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return "", wrapError(CodeUnavailable, "generate_id", "secure random identity generation failed", true, err)
	}
	return prefix + strings.ToLower(idEncoding.EncodeToString(raw)), nil
}

func (id RunID) Validate() error {
	return validateOpaqueID(string(id), "run_", "run_id")
}

func (id AttemptID) Validate() error {
	return validateOpaqueID(string(id), "att_", "attempt_id")
}

func validateOpaqueID(value, prefix, field string) error {
	if len(value) != len(prefix)+encodedIDBytes || !strings.HasPrefix(value, prefix) {
		return invalidField(field, "must be an opaque "+prefix+" base32 128-bit identifier")
	}
	encoded := value[len(prefix):]
	if encoded != strings.ToLower(encoded) {
		return invalidField(field, "must use canonical lowercase base32")
	}
	raw, err := idEncoding.DecodeString(strings.ToUpper(encoded))
	if err != nil || len(raw) != randomIDBytes {
		return invalidField(field, "must contain exactly 128 bits of canonical base32 data")
	}
	if canonical := strings.ToLower(idEncoding.EncodeToString(raw)); canonical != encoded {
		return invalidField(field, fmt.Sprintf("has non-canonical base32 encoding %q", encoded))
	}
	return nil
}
