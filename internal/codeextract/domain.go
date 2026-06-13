package codeextract

type Request struct {
	WorkspaceRoot string
	Reports       []string
}

type Result struct {
	Reports    []ReportResult         `json:"reports"`
	Unresolved []UnresolvedDiagnostic `json:"unresolved,omitempty"`
	Status     Status                 `json:"status"`
}

type Status string

const (
	StatusOK         Status = "ok"
	StatusPartial    Status = "partial"
	StatusValidation Status = "validation"
)

type ReportResult struct {
	Path        string       `json:"path"`
	Sources     []Source     `json:"sources,omitempty"`
	References  []Reference  `json:"references,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

type Source struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Root  string `json:"root"`
}

type Reference struct {
	ReportPath   string      `json:"report_path"`
	SourceName   string      `json:"source_name,omitempty"`
	Citation     string      `json:"citation"`
	CitedPath    string      `json:"cited_path"`
	LineSpec     string      `json:"line_spec"`
	ResolvedPath string      `json:"resolved_path,omitempty"`
	Status       string      `json:"status"`
	Snippet      []Line      `json:"snippet,omitempty"`
	Unresolved   *Diagnostic `json:"unresolved,omitempty"`
}

type Line struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

type Diagnostic struct {
	ReportPath string `json:"report_path,omitempty"`
	SourceName string `json:"source_name,omitempty"`
	Citation   string `json:"citation,omitempty"`
	Path       string `json:"path,omitempty"`
	Reason     string `json:"reason"`
}

type UnresolvedDiagnostic = Diagnostic
