package study

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateStudyArtifactsValidAndInapplicable(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	source := Source{Name: "repo", Kind: SourceKindDirectory, Path: filepath.Join(study.Path, "sources", "repo")}
	doc := Source{Name: "doc.md", Kind: SourceKindMarkdown, Path: filepath.Join(study.Path, "sources", "doc.md"), ApplicableDimensions: []string{"02"}}
	dim := Dimension{Number: "01", Slug: "structure", File: "01-structure.md", Path: filepath.Join(study.Path, "dimensions", "01-structure.md")}
	mkdirStudy(t, study.Path)
	writeReport(t, SourceReportPath(study, source, dim), validSourceValidationReport())
	writeReport(t, FinalReportPath(study, dim), validFinalValidationReport())
	writeReport(t, SummaryPath(study), "source,01-structure,total\nrepo,8,8\n")

	result := ValidateStudyArtifacts(StudyListing{Study: study, Sources: []Source{source, doc}, Dimensions: []Dimension{dim}})
	if result.Status != ValidationStatusPassed {
		t.Fatalf("status = %q, result = %+v", result.Status, result)
	}
	if result.SchemaVersion != StudyValidationSchemaVersion || result.Summary.Failed != 0 || result.Summary.Inapplicable == 0 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	if !hasCheck(result.Checks, "source_dimension.applicability", ValidationStatusInapplicable) {
		t.Fatalf("missing inapplicable check: %+v", result.Checks)
	}
	if !hasCheck(result.Checks, "run_state.parse", ValidationStatusSkipped) {
		t.Fatalf("missing skipped run-state check: %+v", result.Checks)
	}
}

func TestValidateStudyArtifactsMissingAndUnsupportedRunState(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	source := Source{Name: "repo", Kind: SourceKindDirectory, Path: filepath.Join(study.Path, "sources", "repo")}
	dim := Dimension{Number: "01", Slug: "structure", File: "01-structure.md", Path: filepath.Join(study.Path, "dimensions", "01-structure.md")}
	mkdirStudy(t, study.Path)
	path := RunStatePath(study)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":999,"run_id":"r","study":"demo","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ValidateStudyArtifacts(StudyListing{Study: study, Sources: []Source{source}, Dimensions: []Dimension{dim}})
	if result.Status != ValidationStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if !hasCheck(result.Checks, "run_state.parse", ValidationStatusFailed) {
		t.Fatalf("missing failed run-state check: %+v", result.Checks)
	}
	if result.Summary.Failed == 0 {
		t.Fatalf("failed count = 0: %+v", result.Summary)
	}
}

func TestValidateStudyArtifactsDoesNotLeakReportBodySecrets(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	source := Source{Name: "repo", Kind: SourceKindDirectory, Path: filepath.Join(study.Path, "sources", "repo")}
	dim := Dimension{Number: "01", Slug: "structure", File: "01-structure.md", Path: filepath.Join(study.Path, "dimensions", "01-structure.md")}
	mkdirStudy(t, study.Path)
	writeReport(t, SourceReportPath(study, source, dim), "# Report\n\nsk-test-secret\n")

	result := ValidateStudyArtifacts(StudyListing{Study: study, Sources: []Source{source}, Dimensions: []Dimension{dim}})
	if result.Status != ValidationStatusFailed {
		t.Fatalf("status = %q, want failed", result.Status)
	}
	if validationResultContains(result, "sk-test-secret") {
		t.Fatalf("validation result leaked report body secret: %+v", result)
	}
}

func TestValidateStudyServiceReportsMalformedFrontmatter(t *testing.T) {
	root := t.TempDir()
	studyRoot := filepath.Join(root, "studies", "demo")
	mkdirStudy(t, studyRoot)
	if err := os.WriteFile(filepath.Join(studyRoot, "sources", "doc.md"), []byte("---\napplicable_dimensions: [bad\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	_, err := service.ValidateStudy("demo")
	if err == nil {
		t.Fatalf("expected malformed frontmatter error")
	}
}

func validationResultContains(result StudyValidationResult, value string) bool {
	for _, check := range result.Checks {
		if validationCheckContains(check, value) {
			return true
		}
	}
	for _, report := range result.Reports {
		for _, check := range report.Checks {
			if validationCheckContains(check, value) {
				return true
			}
		}
	}
	return false
}

func validationCheckContains(check ValidationCheck, value string) bool {
	combined := strings.Join([]string{check.ID, check.Name, check.Path, check.Expected, check.Observed, check.Guidance}, "\n")
	return strings.Contains(combined, value)
}

func TestValidateStudyArtifactsAcceptsRunState(t *testing.T) {
	root := t.TempDir()
	study := Study{Name: "demo", Path: filepath.Join(root, "studies", "demo")}
	mkdirStudy(t, study.Path)
	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	state := RunState{SchemaVersion: RunStateSchemaVersion, RunID: "run", Study: "demo", CreatedAt: now, UpdatedAt: now}
	if err := SaveRunState(study, state); err != nil {
		t.Fatal(err)
	}
	result := ValidateStudyArtifacts(StudyListing{Study: study})
	if !hasCheck(result.Checks, "run_state.parse", ValidationStatusPassed) {
		t.Fatalf("missing passed run-state check: %+v", result.Checks)
	}
}

func mkdirStudy(t *testing.T, root string) {
	t.Helper()
	for _, rel := range []string{"sources", "dimensions", "reports/source", "reports/final"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func validSourceValidationReport() string {
	return `# Report

## Source Information

## Summary

## Rating

Rating: 8

## Questions and Answers

- Q: one?
  A: answer

code.go:42
`
}

func validFinalValidationReport() string {
	return `# Final Report

## Study Parameters

## Sources Studied

| Source | Path |
| --- | --- |
| repo | sources/repo |

## Executive Summary

## Rating Summary

## Pattern Synthesis

## Open Questions
`
}
