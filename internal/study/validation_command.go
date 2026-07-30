package study

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const StudyValidationSchemaVersion = 1

func (s Service) ValidateStudy(studyRef string) (StudyValidationResult, error) {
	listing, err := s.ListStudy(studyRef)
	if err != nil {
		return StudyValidationResult{}, err
	}
	return ValidateStudyArtifacts(listing), nil
}

func ValidateStudyArtifacts(listing StudyListing) StudyValidationResult {
	result := StudyValidationResult{
		SchemaVersion: StudyValidationSchemaVersion,
		Study:         listing.Study.Name,
		Status:        ValidationStatusPassed,
	}
	addCheck := func(check ValidationCheck) {
		if check.ID == "" {
			check.ID = check.Name
		}
		result.Checks = append(result.Checks, check)
		addValidationCount(&result.Summary, check.Status)
	}
	addReport := func(report ValidationResult) {
		report.SchemaVersion = StudyValidationSchemaVersion
		for i := range report.Checks {
			if report.Checks[i].ID == "" {
				report.Checks[i].ID = report.Checks[i].Name
			}
			addValidationCount(&result.Summary, report.Checks[i].Status)
		}
		result.Reports = append(result.Reports, report)
		if report.Status == ValidationStatusFailed {
			result.Status = ValidationStatusFailed
		}
	}

	addCheck(existsCheck("study.structure.sources", listing.Study.Path, "studies/<study>/sources directory", pathExists(listing.Study.Path, "sources"), "create the study sources directory"))
	addCheck(existsCheck("study.structure.dimensions", listing.Study.Path, "studies/<study>/dimensions directory", pathExists(listing.Study.Path, "dimensions"), "create at least one dimension file"))
	addCheck(existsCheck("study.structure.reports.source", listing.Study.Path, "studies/<study>/reports/source directory", pathExists(listing.Study.Path, "reports", "source"), "run analyses or create the source reports directory"))
	addCheck(existsCheck("study.structure.reports.final", listing.Study.Path, "studies/<study>/reports/final directory", pathExists(listing.Study.Path, "reports", "final"), "run synthesis or create the final report directory"))
	addCheck(existsCheck("summary.csv", listing.Study.Path, "studies/<study>/summary.csv", pathExists(listing.Study.Path, "summary.csv"), "run 'ultraplan study <study> summary' after reports exist"))
	addCheck(validateStudyConfigCheck(listing))

	if len(listing.Sources) == 0 {
		addCheck(failedCheck("source.discovery", listing.Study.Path, "at least one source", "no sources discovered", "", "add directory or Markdown sources under studies/<study>/sources"))
	} else {
		addCheck(passedCheck("source.discovery", listing.Study.Path, ""))
	}
	if len(listing.Dimensions) == 0 {
		addCheck(failedCheck("dimension.discovery", listing.Study.Path, "at least one dimension", "no dimensions discovered", "", "add Markdown dimensions under studies/<study>/dimensions"))
	} else {
		addCheck(passedCheck("dimension.discovery", listing.Study.Path, ""))
	}

	for _, dimension := range listing.Dimensions {
		applicable := map[string]bool{}
		for _, source := range GetApplicableSources(listing.Sources, dimension) {
			applicable[sourceKey(source.Name, source.Kind)] = true
			addReport(ValidateSourceReport(listing.Study, source, dimension))
		}
		for _, source := range listing.Sources {
			if applicable[sourceKey(source.Name, source.Kind)] {
				continue
			}
			check := ValidationCheck{
				ID:         "source_dimension.applicability",
				Name:       "source_dimension.applicability",
				Status:     ValidationStatusInapplicable,
				Severity:   ValidationSeverityInfo,
				Path:       source.Path,
				Expected:   "source applies to dimension " + dimension.Ref(),
				Observed:   "source declares dimension inapplicable",
				SourceKind: source.Kind,
				Guidance:   "no report is required for this source and dimension",
			}
			addCheck(check)
		}
	}
	for _, dimension := range listing.Dimensions {
		addReport(ValidateFinalReport(listing.Study, dimension))
	}
	addCheck(validateRunStateCheck(listing.Study))

	for _, check := range result.Checks {
		if check.Status == ValidationStatusFailed {
			result.Status = ValidationStatusFailed
			break
		}
	}
	if result.Status != ValidationStatusFailed {
		for _, report := range result.Reports {
			if report.Status == ValidationStatusFailed {
				result.Status = ValidationStatusFailed
				break
			}
		}
	}
	return result
}

func validateStudyConfigCheck(listing StudyListing) ValidationCheck {
	path := StudyConfigPath(listing.Study)
	if !listing.Config.Present {
		return ValidationCheck{
			ID:       "study.config",
			Name:     "study.config",
			Status:   ValidationStatusSkipped,
			Severity: ValidationSeverityInfo,
			Path:     path,
			Expected: fmt.Sprintf("optional version %d study config", StudyConfigVersion),
			Observed: "study.json is not present",
			Guidance: "add studies/<study>/study.json only when a custom dimension order is needed",
		}
	}
	return ValidationCheck{
		ID:       "study.config",
		Name:     "study.config",
		Status:   ValidationStatusPassed,
		Severity: ValidationSeverityInfo,
		Path:     path,
		Expected: fmt.Sprintf("version %d study config with valid dimension references", StudyConfigVersion),
		Observed: fmt.Sprintf("%d prioritized dimensions", len(listing.DimensionOrder)),
	}
}

func addValidationCount(counts *ValidationCounts, status ValidationStatus) {
	counts.Total++
	switch status {
	case ValidationStatusPassed:
		counts.Passed++
	case ValidationStatusFailed:
		counts.Failed++
	case ValidationStatusWarning:
		counts.Warnings++
	case ValidationStatusSkipped:
		counts.Skipped++
	case ValidationStatusInapplicable:
		counts.Inapplicable++
	}
}

func existsCheck(name, root, expected string, exists bool, guidance string) ValidationCheck {
	if exists {
		return passedCheck(name, root, "")
	}
	return failedCheck(name, root, expected, "path does not exist", "", guidance)
}

func pathExists(root string, elems ...string) bool {
	parts := append([]string{root}, elems...)
	_, err := os.Stat(filepath.Join(parts...))
	return err == nil
}

func validateRunStateCheck(study Study) ValidationCheck {
	path := RunStatePath(study)
	_, err := LoadRunState(study)
	if err == nil {
		return ValidationCheck{ID: "run_state.parse", Name: "run_state.parse", Status: ValidationStatusPassed, Severity: ValidationSeverityInfo, Path: path}
	}
	status := ValidationStatusFailed
	severity := ValidationSeverityError
	observed := "run state could not be read"
	guidance := "inspect or recreate studies/<study>/.ultraplan/run-state.json"
	switch {
	case errors.Is(err, ErrRunStateMissing):
		status = ValidationStatusSkipped
		severity = ValidationSeverityInfo
		observed = "run state is not present"
		guidance = "run state is optional until run-loop has been used"
	case errors.Is(err, ErrRunStateMalformed):
		observed = "run state is malformed"
	case errors.Is(err, ErrRunStateUnsupported):
		observed = "run state schema version is unsupported"
		guidance = "upgrade or migrate the run state before relying on it"
	}
	return ValidationCheck{
		ID:       "run_state.parse",
		Name:     "run_state.parse",
		Status:   status,
		Severity: severity,
		Path:     path,
		Expected: fmt.Sprintf("schema_version %d run state or no run state", RunStateSchemaVersion),
		Observed: observed,
		Guidance: guidance,
		Err:      err,
	}
}
