package project

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type ProjectReasoningStage string

const (
	ProjectReasoningIndex  ProjectReasoningStage = "index"
	ProjectAreaReasoning   ProjectReasoningStage = "area-reasoning"
	ProjectFinalReasoning  ProjectReasoningStage = "reasoning"
	ProjectReasoningReview ProjectReasoningStage = "review"
)

type ReasoningArea struct {
	Name, Template, Output, Why string
	Required                    bool
	DependsOn                   []string
}
type EvidenceAssignment struct{ Area, Evidence, RelevantQuestions, Why string }
type SourceAssignment struct{ Area, Source, Authority, Why string }
type ExcludedEvidence struct{ Source, Reason, RevisitTrigger string }
type ProjectReasoningManifest struct {
	Areas    []ReasoningArea
	Evidence []EvidenceAssignment
	Sources  []SourceAssignment
	Excluded []ExcludedEvidence
}

type FingerprintRecord struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}
type ReasoningArtifactState struct {
	Stage        ProjectReasoningStage `json:"stage"`
	Output       string                `json:"output"`
	Inputs       []FingerprintRecord   `json:"inputs"`
	OutputSHA256 string                `json:"output_sha256"`
	CompletedAt  time.Time             `json:"completed_at"`
}
type ProjectReasoningState struct {
	SchemaVersion int                               `json:"schema_version"`
	Artifacts     map[string]ReasoningArtifactState `json:"artifacts"`
	Verdict       string                            `json:"verdict,omitempty"`
}
type ProjectReasoningStatus struct {
	Mode            ProjectReasoningMode  `json:"mode"`
	RequiredVerdict string                `json:"required_verdict,omitempty"`
	Accepted        bool                  `json:"accepted"`
	Fresh           bool                  `json:"fresh"`
	Verdict         string                `json:"verdict,omitempty"`
	CurrentStage    ProjectReasoningStage `json:"current_stage"`
	Outputs         []string              `json:"outputs,omitempty"`
	Blockers        []string              `json:"blockers,omitempty"`
}

type ProjectReasoningError struct{ Project, Path, Problem, Recovery string }

func (e ProjectReasoningError) Error() string {
	return fmt.Sprintf("project_reasoning_incomplete:\n%s\n%s\n\nRun:\n%s", e.Path, e.Problem, e.Recovery)
}

func ParseProjectReasoningIndex(content string) (ProjectReasoningManifest, []ValidationFinding) {
	var m ProjectReasoningManifest
	sections := map[string]string{"Reasoning Areas": "areas", "Evidence Assignments": "evidence", "Source Document Assignments": "sources", "Excluded Evidence": "excluded"}
	current := ""
	var headers []string
	seen := map[string]bool{}
	var findings []ValidationFinding
	for i, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") {
			current = sections[strings.TrimSpace(strings.TrimPrefix(t, "## "))]
			headers = nil
			if current != "" {
				seen[current] = true
			}
			continue
		}
		if current == "" || !strings.HasPrefix(t, "|") {
			continue
		}
		cells := parseTableRow(t)
		if len(cells) == 0 || isSeparatorRow(cells) {
			continue
		}
		if headers == nil {
			headers = cells
			continue
		}
		row := rowMap(headers, cells)
		need := func(keys ...string) string {
			for _, k := range keys {
				if v := trimInlineCode(row[strings.ToLower(k)]); v != "" {
					return v
				}
			}
			return ""
		}
		area := need("Area")
		bad := func(msg string) {
			findings = append(findings, ValidationFinding{Severity: SeverityError, Section: CatalogSection("Project Reasoning " + current), Problem: "malformed table row", Cause: fmt.Sprintf("line %d: %s", i+1, msg), Suggestion: "Fill every required column."})
		}
		switch current {
		case "areas":
			template, output := need("Template"), need("Output")
			if area == "" || template == "" || output == "" {
				bad("area, template, and output are required")
				continue
			}
			required := strings.EqualFold(need("Required"), "yes") || strings.EqualFold(need("Required"), "true")
			m.Areas = append(m.Areas, ReasoningArea{Name: area, Template: template, Output: output, Required: required, DependsOn: splitList(need("Depends On")), Why: need("Why")})
		case "evidence":
			if area == "" || need("Evidence") == "" {
				bad("area and evidence are required")
				continue
			}
			m.Evidence = append(m.Evidence, EvidenceAssignment{Area: area, Evidence: need("Evidence"), RelevantQuestions: need("Relevant Questions"), Why: need("Why Assigned")})
		case "sources":
			if area == "" || need("Source") == "" {
				bad("area and source are required")
				continue
			}
			m.Sources = append(m.Sources, SourceAssignment{Area: area, Source: need("Source"), Authority: need("Authority"), Why: need("Why Assigned")})
		case "excluded":
			if need("Source") == "" {
				bad("source is required")
				continue
			}
			m.Excluded = append(m.Excluded, ExcludedEvidence{Source: need("Source"), Reason: need("Reason Excluded"), RevisitTrigger: need("Revisit Trigger")})
		}
	}
	for _, key := range []string{"areas", "evidence", "sources", "excluded"} {
		if !seen[key] {
			findings = append(findings, ValidationFinding{Severity: SeverityError, Problem: "missing project reasoning index section", Cause: key + " section was not found", Suggestion: "Add all required project-reasoning/index.md tables."})
		}
	}
	return m, findings
}

func splitList(s string) []string {
	if s == "" || strings.EqualFold(s, "none") {
		return nil
	}
	f := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
	for i := range f {
		f[i] = strings.TrimSpace(f[i])
	}
	return f
}

func ValidateProjectReasoningManifest(root string, p Project, index ProjectIndex, m ProjectReasoningManifest) []ValidationFinding {
	var out []ValidationFinding
	names := map[string]bool{}
	outputs := map[string]bool{}
	templates := map[string]bool{}
	evidence := map[string]bool{}
	sources := map[string]bool{}
	for _, e := range index.Entries {
		switch e.Section {
		case SectionProjectReasoningTemplates:
			templates[normalizeCatalogPath(e.Path)] = true
		case SectionAvailableEvidenceReports:
			evidence[normalizeCatalogPath(e.Path)] = true
		case SectionSourceDocuments, SectionActiveContractPool:
			sources[normalizeCatalogPath(e.Path)] = true
		}
	}
	add := func(area, path, problem, cause string) {
		out = append(out, ValidationFinding{Severity: SeverityError, Section: "Project Reasoning", EntryName: area, Path: path, Problem: problem, Cause: cause, Suggestion: "Update project-reasoning/index.md with contained, catalogued, acyclic assignments."})
	}
	rootPrefix := "projects/" + p.Name + "/project-reasoning/areas/"
	for _, a := range m.Areas {
		if names[a.Name] {
			add(a.Name, a.Output, "duplicate reasoning area", "area names must be unique")
		}
		names[a.Name] = true
		op := normalizeCatalogPath(a.Output)
		if outputs[op] {
			add(a.Name, op, "duplicate reasoning output", "each area needs its own output")
		}
		outputs[op] = true
		if !strings.HasPrefix(op, rootPrefix) || !strings.HasSuffix(op, ".md") {
			add(a.Name, op, "reasoning output escapes area directory", "outputs must be Markdown under "+rootPrefix)
		}
		if !templates[normalizeCatalogPath(a.Template)] {
			add(a.Name, a.Template, "uncatalogued project reasoning template", "template is absent from Available Project Reasoning Templates")
		}
		validateContainedInput(root, a.Name, a.Template, &out)
	}
	for _, a := range m.Areas {
		for _, d := range a.DependsOn {
			if !names[d] {
				add(a.Name, d, "unknown reasoning dependency", "dependency does not name a selected area")
			}
		}
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	deps := map[string][]string{}
	for _, a := range m.Areas {
		deps[a.Name] = a.DependsOn
	}
	var visit func(string) bool
	visit = func(n string) bool {
		if visiting[n] {
			return true
		}
		if done[n] {
			return false
		}
		visiting[n] = true
		for _, d := range deps[n] {
			if visit(d) {
				return true
			}
		}
		delete(visiting, n)
		done[n] = true
		return false
	}
	for n := range deps {
		if visit(n) {
			add(n, "", "dependency cycle", "selected reasoning areas contain a cycle")
			break
		}
	}
	for _, a := range m.Evidence {
		if !names[a.Area] {
			add(a.Area, a.Evidence, "assignment names unknown area", "")
		}
		if !evidence[normalizeCatalogPath(a.Evidence)] {
			add(a.Area, a.Evidence, "uncatalogued evidence", "evidence is absent from Available Evidence Reports")
		}
		validateContainedInput(root, a.Area, a.Evidence, &out)
	}
	for _, a := range m.Sources {
		if !names[a.Area] {
			add(a.Area, a.Source, "assignment names unknown area", "")
		}
		if !sources[normalizeCatalogPath(a.Source)] {
			add(a.Area, a.Source, "uncatalogued source document", "source is absent from Source Documents or Active Contract Pool")
		}
		validateContainedInput(root, a.Area, a.Source, &out)
	}
	sortFindings(out)
	return out
}

func validateContainedInput(root, area, path string, out *[]ValidationFinding) {
	if isExternalPath(path) {
		return
	}
	if _, err := workspace.ResolveInside(root, normalizeCatalogPath(path)); err != nil {
		*out = append(*out, ValidationFinding{Severity: SeverityError, Section: "Project Reasoning", EntryName: area, Path: path, Problem: "path escapes workspace", Cause: err.Error(), Suggestion: "Use a workspace-relative contained path."})
	}
}

var reasoningRefRE = regexp.MustCompile(`(?im)^\s*-?\s*\*\*Path:\*\*\s*` + "`?" + `([^` + "`" + `\r\n]+)` + "`?" + `\s*$`)
var reasoningLinesRE = regexp.MustCompile(`(?im)^\s*-?\s*\*\*Lines?:\*\*\s*` + "`?" + `([0-9]+(?:-[0-9]+)?)` + "`?" + `\s*$`)

// ResolveReasoningReferences expands explicit Path/Lines references into a bounded prompt packet.
func ResolveReasoningReferences(root, content string) (string, []FingerprintRecord, error) {
	paths := reasoningRefRE.FindAllStringSubmatchIndex(content, -1)
	if len(paths) == 0 {
		return "", nil, nil
	}
	lines := reasoningLinesRE.FindAllStringSubmatchIndex(content, -1)
	var b strings.Builder
	var fps []FingerprintRecord
	for i, m := range paths {
		path := strings.TrimSpace(content[m[2]:m[3]])
		full, err := workspace.ResolveInside(root, filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))))
		if err != nil {
			return "", nil, fmt.Errorf("resolve reasoning reference %s: %w", path, err)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return "", nil, fmt.Errorf("read reasoning reference %s: %w", path, err)
		}
		selected := string(data)
		label := "all"
		if i < len(lines) {
			spec := content[lines[i][2]:lines[i][3]]
			selected, err = selectLines(selected, spec)
			if err != nil {
				return "", nil, fmt.Errorf("resolve reasoning reference %s:%s: %w", path, spec, err)
			}
			label = spec
		}
		sum := sha256.Sum256(data)
		fps = append(fps, FingerprintRecord{Path: path, SHA256: hex.EncodeToString(sum[:])})
		fmt.Fprintf(&b, "\n<<< BEGIN RESOLVED REFERENCE: %s:%s >>>\n%s\n<<< END RESOLVED REFERENCE >>>\n", path, label, selected)
	}
	return b.String(), fps, nil
}
func selectLines(s, spec string) (string, error) {
	parts := strings.Split(spec, "-")
	start, err := strconv.Atoi(parts[0])
	if err != nil || start < 1 {
		return "", errors.New("invalid start line")
	}
	end := start
	if len(parts) == 2 {
		end, err = strconv.Atoi(parts[1])
		if err != nil || end < start {
			return "", errors.New("invalid end line")
		}
	}
	ls := strings.Split(s, "\n")
	if end > len(ls) {
		return "", fmt.Errorf("line %d exceeds file length %d", end, len(ls))
	}
	return strings.Join(ls[start-1:end], "\n"), nil
}

func digestFile(path string) (FingerprintRecord, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return FingerprintRecord{}, err
	}
	sum := sha256.Sum256(data)
	return FingerprintRecord{Path: path, SHA256: hex.EncodeToString(sum[:])}, nil
}
func readReasoningState(path string) (ProjectReasoningState, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return ProjectReasoningState{SchemaVersion: 1, Artifacts: map[string]ReasoningArtifactState{}}, nil
	}
	if err != nil {
		return ProjectReasoningState{}, err
	}
	var s ProjectReasoningState
	if err = json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	if s.Artifacts == nil {
		s.Artifacts = map[string]ReasoningArtifactState{}
	}
	return s, nil
}
func writeReasoningState(path string, s ProjectReasoningState) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".candidate"
	if err = os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func validateReasoningDocument(content string) []string {
	var missing []string
	for _, h := range []string{"Project conclusions", "Trade-Offs", "Evidence", "Risks", "Self-critique"} {
		matched, _ := regexp.MatchString(`(?im)^##\s+`+regexp.QuoteMeta(h)+`\s*$`, content)
		if !matched {
			missing = append(missing, h)
		}
	}
	return missing
}
