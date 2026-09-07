package config

import (
	"strings"
	"testing"
)

func TestRedactTextPreservesEvidenceAndRemovesSecrets(t *testing.T) {
	input := "=== RUN TestWriterToken\nULTRAPLAN_QA_PREDICTED_FAILURE:TestWriterToken\nsecret metadata must be hidden\ntoken=real-token password: \"real password\" Authorization: Bearer access-value\nhttps://user:pass@example.test/ ghp_abcdef1234\nno space left on device"
	got := RedactText(input)
	for _, secret := range []string{"real-token", "real password", "access-value", "user:pass", "ghp_abcdef1234"} {
		if strings.Contains(got, secret) {
			t.Errorf("secret retained: %q", secret)
		}
	}
	for _, evidence := range []string{"TestWriterToken", "ULTRAPLAN_QA_PREDICTED_FAILURE:TestWriterToken", "secret metadata must be hidden", "no space left on device"} {
		if !strings.Contains(got, evidence) {
			t.Errorf("evidence lost: %q", evidence)
		}
	}
}
