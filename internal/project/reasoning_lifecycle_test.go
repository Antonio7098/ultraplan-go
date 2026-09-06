package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

type captureReasoningRuntime struct {
	prompts  []string
	requests []pruntime.Request
}

func (r *captureReasoningRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.prompts = append(r.prompts, req.Prompt)
	r.requests = append(r.requests, req)
	if req.OnEvent != nil {
		req.OnEvent(pruntime.Event{Kind: "message", Type: "text", Payload: map[string]any{"text": "progress"}})
	}
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
	result, err := NewService(root).WithRuntime(rt, pruntime.Request{Provider: "openrouter", Model: "minimax/minimax-m3:free", Timeout: time.Minute}).ReasoningFlow(context.Background(), "p", ProjectAreaReasoning)
	if err != nil {
		t.Fatalf("flow: %v result=%+v", err, result)
	}
	if len(rt.prompts) != 2 {
		t.Fatalf("runtime prompts=%d", len(rt.prompts))
	}
	if rt.requests[1].Provider != "openrouter" || rt.requests[1].Model != "minimax/minimax-m3:free" || rt.requests[1].Timeout != time.Minute {
		t.Fatalf("runtime config not propagated: %+v", rt.requests[1])
	}
	if result.Status.CurrentStage != ProjectFinalReasoning {
		t.Fatalf("current stage = %q, want %q", result.Status.CurrentStage, ProjectFinalReasoning)
	}
	prompt := rt.prompts[1]
	for _, want := range []string{"Kind: assigned-evidence", "Unique study report body", "Relevant questions: Which owner commits state?", "Why assigned: Direct lifecycle evidence"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("area prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestProjectReasoningResultContentAcceptsMarkdownFence(t *testing.T) {
	got := projectReasoningResultContent("```markdown\n# Result\n\nBody.\n```\n")
	if got != "# Result\n\nBody.\n" {
		t.Fatalf("content=%q", got)
	}
}

func TestProjectReasoningTerminalCandidateReplacesExistingOutput(t *testing.T) {
	got, err := projectReasoningCandidate([]byte("old\n"), nil, "```markdown\nnew\n```\n")
	if err != nil || string(got) != "new\n" {
		t.Fatalf("candidate=%q err=%v", got, err)
	}
}

func TestReviewVerdictRequiresConsistentActionableFindingCount(t *testing.T) {
	for _, test := range []struct {
		content string
		valid   bool
	}{
		{"Actionable Findings: 0\nVerdict: pass\n", true},
		{"Actionable Findings: 2\nVerdict: pass_with_findings\n", true},
		{"Actionable Findings: 1\nVerdict: fail\n", true},
		{"Verdict: pass\n", false},
		{"Actionable Findings: 0\nVerdict: pass_with_findings\n", false},
		{"Actionable Findings: 1\nVerdict: pass\n", false},
	} {
		if got := validateReviewVerdict(test.content); (got == nil) != test.valid {
			t.Errorf("validateReviewVerdict(%q)=%v, valid=%v", test.content, got, test.valid)
		}
	}
}

func TestProjectReasoningDirectInputsHaveDeterministicBudget(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"one.md", "two.md"} {
		content := "start-" + name + "\n" + strings.Repeat("x", projectReasoningDirectInputBudget) + "\nend-" + name + "\n"
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	inputs := []projectPromptInput{{ID: "one", Kind: "evidence", Path: "one.md"}, {ID: "two", Kind: "evidence", Path: "two.md"}}
	got, err := NewService(root).appendReasoningInputPacket("instructions", inputs)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) > projectReasoningDirectInputBudget+4096 {
		t.Fatalf("prompt exceeds direct-input budget: %d", len(got))
	}
	for _, want := range []string{"Mode: excerpt", "start-one.md", "end-one.md", "start-two.md", "end-two.md", "ULTRAPLAN OMITTED MIDDLE"} {
		if !strings.Contains(got, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestProjectReasoningStagesShareGovernedPromptPrefix(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "projects", "p")
	if err := os.MkdirAll(filepath.Join(base, "project-reasoning"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"project-index.md":           "## Project Reasoning Policy\n\nstable policy\n",
		"project-reasoning/index.md": "## Reasoning Areas\n\nstable manifest\n",
	} {
		if err := os.WriteFile(filepath.Join(base, filepath.FromSlash(path)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	service := NewService(root)
	project := Project{Name: "p", Path: base}
	shared := []projectPromptInput{
		{ID: "project-index", Kind: "project-index", Path: "projects/p/project-index.md", Assignment: "Authoritative project catalog and reasoning policy."},
		{ID: "project-reasoning-index", Kind: "manifest", Path: "projects/p/project-reasoning/index.md", Assignment: "Selected decision areas, assignments, and dependencies."},
	}
	var prefix string
	for _, stage := range []ProjectReasoningStage{ProjectAreaReasoning, ProjectFinalReasoning, ProjectReasoningReview} {
		prompt, err := service.composeProjectReasoningPrompt(project, ProjectIndex{}, stage, shared, nil)
		if err != nil {
			t.Fatal(err)
		}
		cut := strings.Index(prompt, "## Stage instructions")
		if cut < 0 {
			t.Fatalf("stage prompt missing boundary: %s", prompt)
		}
		got := prompt[:cut]
		if prefix == "" {
			prefix = got
		} else if got != prefix {
			t.Fatalf("stage %s does not reuse the governed prefix", stage)
		}
		if !strings.Contains(got, "stable policy") || !strings.Contains(got, "stable manifest") {
			t.Fatalf("shared prefix lacks governed documents: %s", got)
		}
		if strings.Contains(prompt, "Do not use tools") || !strings.Contains(prompt, "available read-only tools") {
			t.Fatalf("stage %s has incorrect tool-use contract: %s", stage, prompt)
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
