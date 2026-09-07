package config

import "regexp"

var secretTextPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(\b(?:password|passwd|secret|token|api[_-]?key|credential|aws_secret_access_key)\b["']?\s*[:=]\s*)("[^"\r\n]*"|'[^'\r\n]*'|[^\s,;}]+)`),
	regexp.MustCompile(`(?i)(\bbearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`\b(?:sk-|ghp_|github_pat_|xox[baprs]-)[A-Za-z0-9_-]+`),
	regexp.MustCompile(`(https?://)[^\s/@:]+:[^\s/@]+@`),
	regexp.MustCompile(`(?i)(--?(?:token|password|api[_-]?key|secret|credential)\s+)("[^"\r\n]*"|'[^'\r\n]*'|[^\s]+)`),
	regexp.MustCompile(`(?s)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
}

// RedactText preserves diagnostics and test names. RedactValue remains the
// conservative whole-value policy for configuration fields.
func RedactText(value string) string {
	for i, pattern := range secretTextPatterns {
		replacement := "${1}[REDACTED]"
		if i == 2 || i == 5 {
			replacement = "[REDACTED]"
		}
		if i == 3 {
			replacement = "${1}[REDACTED]@"
		}
		value = pattern.ReplaceAllString(value, replacement)
	}
	return value
}
