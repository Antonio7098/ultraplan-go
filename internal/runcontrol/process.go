package runcontrol

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
)

var (
	ErrProcessNotFound            = errors.New("process not found")
	ErrProcessIdentityUnavailable = errors.New("exact process identity unavailable")
)

// NativeProcessProbe resolves process-birth identity using platform-owned
// metadata. Callers must treat ErrProcessIdentityUnavailable as uncertainty.
type NativeProcessProbe struct{}

func (NativeProcessProbe) Probe(ctx context.Context, pid int) (ProcessIdentity, error) {
	return probeNativeProcess(ctx, pid)
}

// NewProcessOwner creates one opaque process-lifetime owner identity and binds
// it to the exact identity of this process where the platform can provide it.
func NewProcessOwner() (Owner, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return Owner{}, wrapError(CodeUnavailable, "owner_identity", "secure process owner identity generation failed", true, err)
	}
	identity, err := probeNativeProcess(context.Background(), os.Getpid())
	if err != nil && !errors.Is(err, ErrProcessIdentityUnavailable) {
		return Owner{}, err
	}
	if identity.PID == 0 {
		identity.PID = os.Getpid()
	}
	encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	owner := Owner{ID: "own_" + strings.ToLower(encoded), Process: identity}
	return owner, owner.Validate()
}

func localHostDigest() string {
	hostname, _ := os.Hostname()
	sum := sha256.Sum256([]byte(hostname))
	return hex.EncodeToString(sum[:16])
}

func validateProbePID(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("%w: pid must be positive", ErrProcessIdentityUnavailable)
	}
	return nil
}
