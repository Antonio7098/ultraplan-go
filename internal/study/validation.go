package study

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var errValidationRead = errors.New("read validation artifact")

func ValidateSourceReport(study Study, source Source, dimension Dimension) ValidationResult {
	path := SourceReportPath(study, source, dimension)
	checks, err := validatePerSourceReport(path, source, dimension)
	status := ValidationStatusPassed
	if err != nil {
		status = ValidationStatusFailed
	}
	return ValidationResult{Kind: ReportKindSource, Path: path, Status: status, Checks: checks, Err: err}
}

func ValidateFinalReport(study Study) ValidationResult {
	path := FinalReportPath(study)
	checks, err := validateFinalReport(path)
	status := ValidationStatusPassed
	if err != nil {
		status = ValidationStatusFailed
	}
	return ValidationResult{Kind: ReportKindFinal, Path: path, Status: status, Checks: checks, Err: err}
}

func validatePerSourceReport(path string, source Source, dimension Dimension) ([]ValidationCheck, error) {
	content, err := readValidationFile(path)
	if err != nil {
		return []ValidationCheck{fileCheck(path, source.Kind, err)}, err
	}
	var checks []ValidationCheck
	var failed bool
	if len(strings.TrimSpace(content)) == 0 {
		checks = append(checks, failedCheck("content.non_empty", path, "non-empty report", "empty file", source.Kind, "regenerate the report"))
		return checks, fmt.Errorf("%w: %s is empty", errValidationRead, path)
	}
	checks = append(checks, passedCheck("content.non_empty", path, source.Kind))
	required := []struct {
		name     string
		values   []string
		guidance string
	}{
		{"heading.top_level", []string{"# "}, "add a top-level report heading"},
		{"section.source_info", []string{"source info", "source information"}, "add a source information section"},
		{"section.summary", []string{"summary"}, "add a summary section"},
		{"section.rating", []string{"rating"}, "add a rating section"},
		{"section.qa", []string{"question", "answer"}, "add question and answer content"},
	}
	for _, req := range required {
		if !containsSection(content, req.values...) {
			checks = append(checks, failedCheck(req.name, path, req.values[0], "missing section", source.Kind, req.guidance))
			failed = true
		} else {
			checks = append(checks, passedCheck(req.name, path, source.Kind))
		}
	}
	rating := findRating(content)
	switch rating.State {
	case RatingStateValid:
		checks = append(checks, ValidationCheck{Name: "rating.parse", Status: ValidationStatusPassed, Severity: ValidationSeverityInfo, Path: path, SourceKind: source.Kind, Observed: fmt.Sprintf("%d", rating.Score)})
	case RatingStateMissing:
		checks = append(checks, ValidationCheck{Name: "rating.parse", Status: ValidationStatusFailed, Severity: ValidationSeverityError, Path: path, Expected: "parseable rating", Observed: "missing rating", SourceKind: source.Kind, Guidance: "add a rating to the report"})
		failed = true
	case RatingStateAmbiguous:
		checks = append(checks, ValidationCheck{Name: "rating.parse", Status: ValidationStatusFailed, Severity: ValidationSeverityWarn, Path: path, Expected: "one rating", Observed: rating.Reason, SourceKind: source.Kind, Guidance: "remove conflicting rating values"})
		failed = true
	default:
		checks = append(checks, ValidationCheck{Name: "rating.parse", Status: ValidationStatusFailed, Severity: ValidationSeverityError, Path: path, Expected: "parseable rating", Observed: rating.Reason, SourceKind: source.Kind, Guidance: "use a supported rating format"})
		failed = true
	}
	if source.Kind == SourceKindDirectory && !dimension.DisableCodeCitations {
		if !citationShapeValid(content) {
			checks = append(checks, failedCheck("citation.shape", path, "file.go:42", "citation shape missing", source.Kind, "use file paths and line numbers"))
			failed = true
		} else {
			checks = append(checks, passedCheck("citation.shape", path, source.Kind))
		}
	} else {
		checks = append(checks, ValidationCheck{Name: "citation.shape", Status: ValidationStatusSkipped, Severity: ValidationSeverityInfo, Path: path, SourceKind: source.Kind, Guidance: "code citations are not required for this source and dimension"})
	}
	if failed {
		return checks, fmt.Errorf("%w: %s failed validation", errValidationRead, path)
	}
	return checks, nil
}

func validateFinalReport(path string) ([]ValidationCheck, error) {
	content, err := readValidationFile(path)
	if err != nil {
		return []ValidationCheck{fileCheck(path, SourceKindMarkdown, err)}, err
	}
	var checks []ValidationCheck
	var failed bool
	if len(strings.TrimSpace(content)) == 0 {
		checks = append(checks, failedCheck("content.non_empty", path, "non-empty report", "empty file", "", "regenerate the final report"))
		return checks, fmt.Errorf("%w: %s is empty", errValidationRead, path)
	}
	checks = append(checks, passedCheck("content.non_empty", path, ""))
	for _, req := range []struct {
		name     string
		aliases  []string
		guidance string
	}{
		{"section.study_context", []string{"study parameters", "study context"}, "add study parameters or equivalent context"},
		{"section.sources_table", []string{"sources studied", "| source |", "| sources |"}, "add a sources studied table"},
		{"section.executive_summary", []string{"executive summary"}, "add an executive summary"},
		{"section.rating_summary", []string{"rating summary"}, "add a rating summary"},
		{"section.synthesis", []string{"pattern", "synthesis"}, "add pattern or synthesis content"},
		{"section.open_questions", []string{"open questions", "notable absences"}, "add open questions or notable absences"},
	} {
		if !containsSection(content, req.aliases...) {
			checks = append(checks, failedCheck(req.name, path, req.aliases[0], "missing section", "", req.guidance))
			failed = true
		} else {
			checks = append(checks, passedCheck(req.name, path, ""))
		}
	}
	if failed {
		return checks, fmt.Errorf("%w: %s failed validation", errValidationRead, path)
	}
	return checks, nil
}

func readValidationFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%w: %s: %w", errValidationRead, path, err)
	}
	return string(content), nil
}

func fileCheck(path string, kind SourceKind, err error) ValidationCheck {
	return ValidationCheck{
		Name:       "content.read",
		Status:     ValidationStatusFailed,
		Severity:   ValidationSeverityError,
		Path:       path,
		Observed:   validationErrorSummary(err),
		SourceKind: kind,
		Guidance:   "ensure the report file exists and is readable",
		Err:        err,
	}
}

func passedCheck(name, path string, kind SourceKind) ValidationCheck {
	return ValidationCheck{Name: name, Status: ValidationStatusPassed, Severity: ValidationSeverityInfo, Path: path, SourceKind: kind}
}

func failedCheck(name, path, expected, observed string, kind SourceKind, guidance string) ValidationCheck {
	return ValidationCheck{Name: name, Status: ValidationStatusFailed, Severity: ValidationSeverityError, Path: path, Expected: expected, Observed: observed, SourceKind: kind, Guidance: guidance}
}

func findRating(content string) RatingResult {
	lines := strings.Split(content, "\n")
	var ratingLines []string
	for _, line := range lines {
		if ratingFractionPattern.MatchString(line) || ratingLabelPattern.MatchString(line) {
			ratingLines = append(ratingLines, line)
		}
	}
	if len(ratingLines) > 0 {
		return ParseRating(strings.Join(ratingLines, "\n"))
	}
	return RatingResult{State: RatingStateMissing}
}

func containsSection(content string, needles ...string) bool {
	lower := strings.ToLower(content)
	for _, needle := range needles {
		if strings.Contains(lower, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func citationShapeValid(content string) bool {
	return regexp.MustCompile(`\b[a-zA-Z0-9_.\-/]+\.(?:go|md|txt):\d+(?:-\d+)?\b`).FindString(content) != ""
}

func validationErrorSummary(err error) string {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return "file does not exist"
	case errors.Is(err, os.ErrPermission):
		return "file is not readable"
	default:
		return "file could not be read"
	}
}
