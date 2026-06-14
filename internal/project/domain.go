package project

type Project struct {
	Name string
	Path string
}

type CatalogSection string

const (
	SectionSourceDocuments            CatalogSection = "Source Documents"
	SectionActiveContractPool         CatalogSection = "Active Contract Pool"
	SectionAvailableEvidenceReports   CatalogSection = "Available Evidence Reports"
	SectionAvailableReasoningTemplate CatalogSection = "Available Reasoning Templates"
	SectionReviewProtocols            CatalogSection = "Review Protocols"
)

type ProjectIndex struct {
	Entries []CatalogEntry
}

type CatalogEntry struct {
	Section     CatalogSection
	Name        string
	Path        string
	Description string
	External    bool
}

type StatusState string

const (
	StatusPresent StatusState = "present"
	StatusMissing StatusState = "missing"
	StatusEmpty   StatusState = "empty"
	StatusInvalid StatusState = "invalid"
	StatusOK      StatusState = "ok"
)

type ProjectStatus struct {
	Project         Project
	DocsDir         StatusState
	MarkdownDocs    []string
	Roadmap         StatusState
	ProjectIndex    StatusState
	SprintsDir      StatusState
	SprintDirs      []string
	Catalog         StatusState
	ValidationFinds []ValidationFinding
}

type ValidationSeverity string

const (
	SeverityError ValidationSeverity = "error"
	SeverityWarn  ValidationSeverity = "warn"
)

type ValidationFinding struct {
	Severity   ValidationSeverity
	Section    CatalogSection
	EntryName  string
	Path       string
	Problem    string
	Cause      string
	Suggestion string
	Err        error
}

type ValidationResult struct {
	Project  Project
	Status   StatusState
	Findings []ValidationFinding
}
