package study

const (
	RunStateSchemaVersion = 1
	RunStateDirName       = ".ultraplan"
	RunStateFileName      = "run-state.json"
)

type Study struct {
	Name string
	Path string
}

type ReportKind string

const (
	ReportKindSource ReportKind = "source"
	ReportKindFinal  ReportKind = "final"
)

type SourceKind string

const (
	SourceKindDirectory SourceKind = "directory"
	SourceKindMarkdown  SourceKind = "markdown"
)

type Source struct {
	Name                 string
	Kind                 SourceKind
	Path                 string
	ApplicableDimensions []string
	Frontmatter          map[string]any
}

type Dimension struct {
	Number               string
	Slug                 string
	File                 string
	Path                 string
	DisableCodeCitations bool
}

type PromptKind string

const (
	PromptKindDirectoryAnalysis PromptKind = "directory_analysis"
	PromptKindMarkdownAnalysis  PromptKind = "markdown_analysis"
	PromptKindSynthesis         PromptKind = "synthesis"
)

type PromptRequest struct {
	WorkspaceRoot string
	Study         Study
	Dimension     Dimension
	Source        Source
	Sources       []Source
}

type PromptManifest struct {
	Kind               PromptKind          `json:"kind"`
	Study              string              `json:"study"`
	Dimension          string              `json:"dimension"`
	Source             string              `json:"source,omitempty"`
	SourceKind         SourceKind          `json:"source_kind,omitempty"`
	Templates          []string            `json:"templates"`
	DimensionPath      string              `json:"dimension_path"`
	InputDocumentPath  string              `json:"input_document_path,omitempty"`
	InputReportPaths   []string            `json:"input_report_paths,omitempty"`
	SourceReports      []SourceReportInput `json:"source_reports,omitempty"`
	ExpectedOutputPath string              `json:"expected_output_path"`
}

type SourceReportInput struct {
	Source     string     `json:"source"`
	SourceKind SourceKind `json:"source_kind"`
	Path       string     `json:"path"`
}

type PromptResult struct {
	Text     string
	Manifest PromptManifest
}

type ValidationStatus string

const (
	ValidationStatusPassed       ValidationStatus = "passed"
	ValidationStatusFailed       ValidationStatus = "failed"
	ValidationStatusWarning      ValidationStatus = "warning"
	ValidationStatusSkipped      ValidationStatus = "skipped"
	ValidationStatusInapplicable ValidationStatus = "inapplicable"
)

type ValidationSeverity string

const (
	ValidationSeverityInfo  ValidationSeverity = "info"
	ValidationSeverityWarn  ValidationSeverity = "warn"
	ValidationSeverityError ValidationSeverity = "error"
)

type ValidationCheck struct {
	ID         string             `json:"id,omitempty"`
	Name       string             `json:"name"`
	Status     ValidationStatus   `json:"status"`
	Severity   ValidationSeverity `json:"severity"`
	Path       string             `json:"path,omitempty"`
	Expected   string             `json:"expected,omitempty"`
	Observed   string             `json:"observed,omitempty"`
	SourceKind SourceKind         `json:"source_kind,omitempty"`
	Guidance   string             `json:"guidance,omitempty"`
	Err        error              `json:"-"`
}

type ValidationResult struct {
	SchemaVersion int               `json:"schema_version,omitempty"`
	Kind          ReportKind        `json:"kind"`
	Path          string            `json:"path"`
	Status        ValidationStatus  `json:"status"`
	Checks        []ValidationCheck `json:"checks"`
	Err           error             `json:"-"`
}

type StudyValidationResult struct {
	SchemaVersion int                `json:"schema_version"`
	Study         string             `json:"study"`
	Status        ValidationStatus   `json:"status"`
	Summary       ValidationCounts   `json:"summary"`
	Checks        []ValidationCheck  `json:"checks"`
	Reports       []ValidationResult `json:"reports"`
}

type ValidationCounts struct {
	Passed       int `json:"passed"`
	Failed       int `json:"failed"`
	Warnings     int `json:"warnings"`
	Skipped      int `json:"skipped"`
	Inapplicable int `json:"inapplicable"`
	Total        int `json:"total"`
}

type RatingState string

const (
	RatingStateValid     RatingState = "valid"
	RatingStateMissing   RatingState = "missing"
	RatingStateInvalid   RatingState = "invalid"
	RatingStateAmbiguous RatingState = "ambiguous"
)

type RatingResult struct {
	State  RatingState `json:"state"`
	Score  int         `json:"score,omitempty"`
	Raw    string      `json:"raw,omitempty"`
	Reason string      `json:"reason,omitempty"`
}

func (d Dimension) Ref() string {
	if d.Slug == "" {
		return d.Number
	}
	return d.Number + "-" + d.Slug
}
