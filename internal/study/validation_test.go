package study

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateSourceReport(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	source := Source{Name: "repo", Kind: SourceKindDirectory}
	dim := Dimension{Number: "01", Slug: "structure"}
	reportPath := SourceReportPath(study, source, dim)
	writeReport(t, reportPath, `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer

code.go:42
`)
	res := ValidateSourceReport(study, source, dim)
	if res.Status != ValidationStatusPassed {
		t.Fatalf("Status = %q, want passed: %+v", res.Status, res)
	}
	if res.Path != reportPath {
		t.Fatalf("Path = %q, want %q", res.Path, reportPath)
	}
	if res.Checks[len(res.Checks)-1].Name != "citation.shape" || res.Checks[len(res.Checks)-1].Status != ValidationStatusPassed {
		t.Fatalf("final check = %+v", res.Checks[len(res.Checks)-1])
	}
}

func TestValidateSourceReportAcceptsCommonCodeCitationExtensions(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	source := Source{Name: "repo", Kind: SourceKindDirectory}

	for _, tc := range []struct {
		name     string
		citation string
	}{
		{name: "python", citation: "lib/crewai/src/crewai/flow/runtime/__init__.py:2984"},
		{name: "typescript", citation: "packages/core/src/runtime.ts:41-53"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dim := Dimension{Number: "01-" + tc.name, Slug: "structure"}
			reportPath := SourceReportPath(study, source, dim)
			writeReport(t, reportPath, `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer

`+tc.citation+`
`)
			res := ValidateSourceReport(study, source, dim)
			if res.Status != ValidationStatusPassed {
				t.Fatalf("Status = %q, want passed: %+v", res.Status, res)
			}
			if !hasCheck(res.Checks, "citation.shape", ValidationStatusPassed) {
				t.Fatalf("missing passed citation check: %+v", res.Checks)
			}
		})
	}
}

func TestValidateSourceReportFailures(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	source := Source{Name: "doc.md", Kind: SourceKindMarkdown}
	dim := Dimension{Number: "01", Slug: "structure"}
	reportPath := SourceReportPath(study, source, dim)
	t.Run("missing", func(t *testing.T) {
		res := ValidateSourceReport(study, source, dim)
		if res.Status != ValidationStatusFailed || !errors.Is(res.Err, os.ErrNotExist) {
			t.Fatalf("res = %+v", res)
		}
		if res.Checks[0].Name != "content.read" {
			t.Fatalf("check[0] = %+v", res.Checks[0])
		}
		if res.Checks[0].Observed != "file does not exist" {
			t.Fatalf("Observed = %q", res.Checks[0].Observed)
		}
	})
	writeReport(t, reportPath, "")
	res := ValidateSourceReport(study, source, dim)
	if res.Status != ValidationStatusFailed {
		t.Fatalf("Status = %q, want failed", res.Status)
	}
	if !errors.Is(res.Err, errValidationRead) {
		t.Fatalf("Err = %v, want read validation error", res.Err)
	}
	if got := res.Checks[0].Name; got != "content.non_empty" {
		t.Fatalf("check[0] = %q", got)
	}
	writeReport(t, reportPath, `# Report

## Source Info

## Summary

## Rating

Rating: 8 and 7/10
`)
	res = ValidateSourceReport(study, source, dim)
	if !hasCheck(res.Checks, "rating.parse", ValidationStatusFailed) {
		t.Fatalf("missing failed rating check: %+v", res.Checks)
	}
	writeReport(t, reportPath, `# Report

## Source Info

## Summary

## Rating

Rating: 7
Rating: 9
`)
	res = ValidateSourceReport(study, source, dim)
	if !hasCheck(res.Checks, "rating.parse", ValidationStatusFailed) {
		t.Fatalf("missing failed rating check for separate rating lines: %+v", res.Checks)
	}
	writeReport(t, reportPath, `# Report

## Source Info

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer
`)
	res = ValidateSourceReport(study, Source{Name: "doc.md", Kind: SourceKindMarkdown}, dim)
	if got := res.Checks[len(res.Checks)-1].Status; got != ValidationStatusSkipped {
		t.Fatalf("citation check = %+v", res.Checks[len(res.Checks)-1])
	}
}

func TestValidateSourceReportMissingSectionsAndCitationPolicy(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	dim := Dimension{Number: "01", Slug: "structure"}
	source := Source{Name: "repo", Kind: SourceKindDirectory}
	reportPath := SourceReportPath(study, source, dim)
	writeReport(t, reportPath, `# Report

## Source Information
`)
	res := ValidateSourceReport(study, source, dim)
	for _, name := range []string{"section.summary", "section.rating", "section.qa", "rating.parse", "citation.shape"} {
		if !hasCheck(res.Checks, name, ValidationStatusFailed) {
			t.Fatalf("missing failed check %q: %+v", name, res.Checks)
		}
	}

	disabledDim := Dimension{Number: "02", Slug: "no-citations", DisableCodeCitations: true}
	disabledPath := SourceReportPath(study, source, disabledDim)
	writeReport(t, disabledPath, `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer
`)
	res = ValidateSourceReport(study, source, disabledDim)
	if res.Status != ValidationStatusPassed {
		t.Fatalf("Status = %q, want passed: %+v", res.Status, res.Checks)
	}
	if !hasCheck(res.Checks, "citation.shape", ValidationStatusSkipped) {
		t.Fatalf("missing skipped citation check: %+v", res.Checks)
	}
}

func hasCheck(checks []ValidationCheck, name string, status ValidationStatus) bool {
	for _, check := range checks {
		if check.Name == name && check.Status == status {
			return true
		}
	}
	return false
}

func TestValidateFinalReport(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	reportPath := FinalReportPath(study)
	writeReport(t, reportPath, `# Final Report

## Study Parameters

## Sources Studied

| Source | Path |
| --- | --- |
| repo | sources/repo |

## Executive Summary

## Rating Summary

## Pattern Synthesis

## Open Questions
`)
	res := ValidateFinalReport(study)
	if res.Status != ValidationStatusPassed {
		t.Fatalf("Status = %q, want passed: %+v", res.Status, res)
	}
}

func TestValidateFinalReportFailures(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	reportPath := FinalReportPath(study)
	writeReport(t, reportPath, `# Final Report

## Executive Summary
`)
	res := ValidateFinalReport(study)
	if res.Status != ValidationStatusFailed {
		t.Fatalf("Status = %q, want failed", res.Status)
	}
	if res.Checks[0].Name != "content.non_empty" {
		t.Fatalf("check[0] = %+v", res.Checks[0])
	}
	for _, name := range []string{"section.study_context", "section.sources_table", "section.rating_summary", "section.synthesis", "section.open_questions"} {
		if !hasCheck(res.Checks, name, ValidationStatusFailed) {
			t.Fatalf("missing failed check %q: %+v", name, res.Checks)
		}
	}
}

func writeReport(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
