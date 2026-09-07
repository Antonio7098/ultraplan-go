package sprint

import (
	"strings"
	"testing"
)

func TestQAReplayIdentityAllowsPolicyMigrationOnly(t *testing.T) {
	retained := QAMap{
		Project: "alpha", Sprint: "39-performance-stage",
		GovernedInputFingerprint: strings.Repeat("a", 64), ImplementationFingerprint: strings.Repeat("b", 64),
		ReviewFingerprint: strings.Repeat("c", 64), CheckCatalogFingerprint: strings.Repeat("d", 64),
		PolicyFingerprint: strings.Repeat("e", 64), Target: QATargetIdentity{Fingerprint: strings.Repeat("f", 64)},
	}
	current := retained
	current.PolicyFingerprint = strings.Repeat("1", 64)
	if err := validateQAReplayIdentity(retained, current); err != nil {
		t.Fatalf("policy-only migration was rejected: %v", err)
	}
	current.ImplementationFingerprint = strings.Repeat("2", 64)
	if err := validateQAReplayIdentity(retained, current); err == nil {
		t.Fatal("implementation drift was accepted")
	}
}
