package project

type Project struct {
	Name string
	Path string
}

type CatalogSection string

const (
	SectionProjectReasoningPolicy     CatalogSection = "Project Reasoning Policy"
	SectionProjectReasoningTemplates  CatalogSection = "Available Project Reasoning Templates"
	SectionSourceDocuments            CatalogSection = "Source Documents"
	SectionActiveContractPool         CatalogSection = "Active Contract Pool"
	SectionAvailableEvidenceReports   CatalogSection = "Available Evidence Reports"
	SectionAvailableReasoningTemplate CatalogSection = "Available Reasoning Templates"
	SectionReviewProtocols            CatalogSection = "Review Protocols"
	SectionSmokeHarnesses             CatalogSection = "Smoke Harnesses"
)

type ProjectIndex struct {
	Entries                []CatalogEntry
	ProjectReasoningPolicy ProjectReasoningPolicy
}

type ProjectReasoningMode string

const (
	ProjectReasoningOptional ProjectReasoningMode = "optional"
	ProjectReasoningRequired ProjectReasoningMode = "required"
)

type ProjectReasoningPolicy struct {
	Mode                  ProjectReasoningMode
	RequiredReviewVerdict string
}

type CatalogEntry struct {
	Section     CatalogSection
	Name        string
	Path        string
	Description string
	External    bool
	Manifest    string
	Evidence    []string
	Status      string
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
	Project                  Project
	DocsDir                  StatusState
	MarkdownDocs             []string
	Roadmap                  StatusState
	ProjectIndex             StatusState
	SprintsDir               StatusState
	SprintDirs               []string
	Catalog                  StatusState
	ReasoningDefaults        []ReasoningDefault
	SprintReasoningTemplates []string
	// AreaReasoningDocuments remains populated for API compatibility.
	AreaReasoningDocuments []string
	ProjectReasoning       ProjectReasoningStatus
	ValidationFinds        []ValidationFinding
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
