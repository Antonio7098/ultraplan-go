package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
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

type captureReasoningRuntime struct{ prompts []string }

func (r *captureReasoningRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.prompts = append(r.prompts, req.Prompt)
	return pruntime.Result{Status: "success"}, nil
}

func TestAreaFlowDirectlyInjectsAssignedStudyReport(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "projects", "p")
	for _, dir := range []string{"templates", "evidence", "project-reasoning/areas"} {
		if err := os.MkdirAll(filepath.Join(p, filepath.FromSlash(dir)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	projectIndex := `## Available Project Reasoning Templates

| Template | Path | Use When |
| --- | --- | --- |
| Lifecycle | projects/p/templates/lifecycle.md | lifecycle decisions |

## Available Evidence Reports

| Report | Path | Covers |
| --- | --- | --- |
| Study | projects/p/evidence/study.md | lifecycle evidence |
`
	manifest := `## Reasoning Areas

| Area | Template | Output | Required | Depends On | Why |
| --- | --- | --- | --- | --- | --- |
| Lifecycle | projects/p/templates/lifecycle.md | projects/p/project-reasoning/areas/lifecycle.md | yes | none | Own lifecycle decisions |

## Evidence Assignments

| Area | Evidence | Relevant Questions | Why Assigned |
| --- | --- | --- | --- |
| Lifecycle | projects/p/evidence/study.md | Which owner commits state? | Direct lifecycle evidence |

## Source Document Assignments

| Area | Source | Authority | Why Assigned |
| --- | --- | --- | --- |

## Excluded Evidence

| Source | Reason Excluded | Revisit Trigger |
| --- | --- | --- |
`
	area := "## Project conclusions\n\nConclusion.\n\n## Trade-Offs\n\nTrade.\n\n## Evidence\n\nEvidence.\n\n## Risks\n\nRisk.\n\n## Self-critique\n\nCritique.\n"
	for path, content := range map[string]string{"project-index.md": projectIndex, "templates/lifecycle.md": "# Lifecycle template\n", "evidence/study.md": "# Unique study report body\n\nState ownership evidence.\n", "project-reasoning/index.md": manifest, "project-reasoning/areas/lifecycle.md": area} {
		if err := os.WriteFile(filepath.Join(p, filepath.FromSlash(path)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	rt := &captureReasoningRuntime{}
	result, err := NewService(root).WithRuntime(rt).ReasoningFlow(context.Background(), "p", ProjectAreaReasoning)
	if err != nil {
		t.Fatalf("flow: %v result=%+v", err, result)
	}
	if len(rt.prompts) != 2 {
		t.Fatalf("runtime prompts=%d", len(rt.prompts))
	}
	prompt := rt.prompts[1]
	for _, want := range []string{"Kind: assigned-evidence", "Unique study report body", "Relevant questions: Which owner commits state?", "Why assigned: Direct lifecycle evidence"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("area prompt missing %q:\n%s", want, prompt)
		}
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
