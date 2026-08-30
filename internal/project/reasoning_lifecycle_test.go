package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectIndexParsesReasoningPolicyAndTemplates(t *testing.T) {
	content := `## Project Reasoning Policy

| Setting | Value |
| --- | --- |
| Mode | required |
| Required Review Verdict | pass_with_findings |

## Available Project Reasoning Templates

| Template | Path | Use When |
| --- | --- | --- |
| Lifecycle | templates/lifecycle.md | lifecycle decisions |
`
	index, findings := ParseProjectIndex(content)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	if index.ProjectReasoningPolicy.Mode != ProjectReasoningRequired || index.ProjectReasoningPolicy.RequiredReviewVerdict != "pass_with_findings" {
		t.Fatalf("policy = %+v", index.ProjectReasoningPolicy)
	}
	if len(index.Entries) != 1 || index.Entries[0].Section != SectionProjectReasoningTemplates {
		t.Fatalf("entries = %+v", index.Entries)
	}
}

func TestProjectReasoningManifestRejectsCycleAndDuplicateOutput(t *testing.T) {
	root := t.TempDir()
	projectRoot := filepath.Join(root, "projects", "p")
	_ = os.MkdirAll(filepath.Join(projectRoot, "templates"), 0o755)
	_ = os.WriteFile(filepath.Join(projectRoot, "templates", "a.md"), []byte("template"), 0o644)
	index := ProjectIndex{Entries: []CatalogEntry{{Section: SectionProjectReasoningTemplates, Name: "A", Path: "projects/p/templates/a.md"}}}
	m := ProjectReasoningManifest{Areas: []ReasoningArea{{Name: "a", Template: "projects/p/templates/a.md", Output: "projects/p/project-reasoning/areas/same.md", DependsOn: []string{"b"}}, {Name: "b", Template: "projects/p/templates/a.md", Output: "projects/p/project-reasoning/areas/same.md", DependsOn: []string{"a"}}}}
	findings := ValidateProjectReasoningManifest(root, Project{Name: "p", Path: projectRoot}, index, m)
	joined := ""
	for _, f := range findings {
		joined += f.Problem + "\n"
	}
	if !strings.Contains(joined, "duplicate reasoning output") || !strings.Contains(joined, "dependency cycle") {
		t.Fatalf("findings = %+v", findings)
	}
}

func TestResolveReasoningReferencesContainsPathsAndSelectsLines(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "evidence"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "evidence", "report.md"), []byte("one\ntwo\nthree\n"), 0o644)
	packet, fps, err := ResolveReasoningReferences(root, "### Report\n\n**Path:** `evidence/report.md`\n**Lines:** `2-3`\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(packet, "two\nthree") || strings.Contains(packet, "\none\n") || len(fps) != 1 {
		t.Fatalf("packet=%q fps=%+v", packet, fps)
	}
}

func TestRequiredProjectReasoningFailsClosedBeforeSprintCreation(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "projects", "p")
	_ = os.MkdirAll(filepath.Join(p, "docs"), 0o755)
	_ = os.MkdirAll(filepath.Join(p, "sprints"), 0o755)
	_ = os.WriteFile(filepath.Join(p, "project-index.md"), []byte("## Project Reasoning Policy\n\n| Setting | Value |\n| --- | --- |\n| Mode | required |\n| Required Review Verdict | pass |\n"), 0o644)
	err := NewService(root).RequireAcceptedReasoning("p")
	var typed ProjectReasoningError
	if err == nil || !strings.Contains(err.Error(), "ultraplan project p reasoning flow --to review") || !strings.Contains(err.Error(), "project_reasoning_incomplete") {
		t.Fatalf("error = %v typed=%+v", err, typed)
	}
}
