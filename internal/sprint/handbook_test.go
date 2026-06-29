package sprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/project"
)

func TestTechnicalHandbookValidationAndManifest(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\n\nDistill stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	catalog, _ := project.ParseProjectIndex(testProjectIndex())
	inputs, err := NewFSStore(root).ReadPlanningInputs(sp)
	if err != nil {
		t.Fatal(err)
	}
	manifest, findings := BuildHandbookManifest(root, sp, inputs, catalog)
	if len(findings) != 0 {
		t.Fatalf("manifest findings = %+v", findings)
	}
	if len(manifest.Evidence) != 1 || manifest.Evidence[0].RelPath != "studies/go-cli-study/reports/final/01-project-structure.md" {
		t.Fatalf("manifest = %+v", manifest)
	}
	if got := ValidateTechnicalHandbookContent(validTechnicalHandbook(), manifest); len(got) != 0 {
		t.Fatalf("valid handbook findings = %+v", got)
	}
	got := ValidateTechnicalHandbookContent(strings.Replace(validTechnicalHandbook(), "## Evidence Pointers", "## Final Decisions", 1), manifest)
	if len(got) == 0 {
		t.Fatalf("expected decision-language finding")
	}
	got = ValidateTechnicalHandbookContent(strings.Replace(validTechnicalHandbook(), "01-project-structure", "08-concurrency", -1), manifest)
	if len(got) == 0 {
		t.Fatalf("expected missing selected evidence trace")
	}
}

func TestTechnicalHandbookPromptDryRunAndFlow(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\n\nDistill stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")

	service := NewService(root).WithRuntime(panicRuntime{})
	preview, err := service.PromptTechnicalHandbook("proj", "01")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Create Technical Handbook", "Prompt source: `builtin:prompts/create-technical-handbook.md`", "Selected evidence:", "Do not make final implementation decisions", "projects/proj/sprints/01-alpha/technical-handbook.md"} {
		if !strings.Contains(preview.Prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, preview.Prompt)
		}
	}
	result, err := service.FlowTechnicalHandbook(context.Background(), "proj", "01", FlowRequest{To: StageTechnicalHandbook, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.DryRun || result.Message == "" {
		t.Fatalf("dry run result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote flow state: %v", err)
	}

	service = NewService(root).WithRuntime(writeHandbookRuntime{root: root, sp: sp, content: validTechnicalHandbook()})
	result, err = service.FlowTechnicalHandbook(context.Background(), "proj", "01", FlowRequest{To: StageTechnicalHandbook})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stages[2].Status != StatusComplete || result.Stages[3].Status != StatusReady {
		t.Fatalf("stages = %+v", result.Stages)
	}
}

func TestSelectedEvidenceRejectsUnsafeAndUnreadablePaths(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFileContent(t, sp.Path, "# Requirements\n\nDistill stage.\n", "requirements.md")
	writeFileContent(t, sp.Path, strings.Replace(validSprintIndex(), ".ultra/studies/go-cli-study/reports/final/01-project-structure.md", "../outside.md", 1), "sprint-index.md")
	catalog, _ := project.ParseProjectIndex(strings.Replace(testProjectIndex(), ".ultra/studies/go-cli-study/reports/final/01-project-structure.md", "../outside.md", 1))
	inputs, err := NewFSStore(root).ReadPlanningInputs(sp)
	if err != nil {
		t.Fatal(err)
	}
	_, findings := BuildHandbookManifest(root, sp, inputs, catalog)
	if len(findings) == 0 {
		t.Fatalf("expected unsafe evidence finding")
	}

	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	catalog, _ = project.ParseProjectIndex(testProjectIndex())
	inputs, err = NewFSStore(root).ReadPlanningInputs(sp)
	if err != nil {
		t.Fatal(err)
	}
	_, findings = BuildHandbookManifest(root, sp, inputs, catalog)
	if len(findings) == 0 {
		t.Fatalf("expected unreadable evidence finding")
	}
}

type writeHandbookRuntime struct {
	root    string
	sp      Sprint
	content string
}

func (r writeHandbookRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	path := filepath.Join(r.sp.Path, "technical-handbook.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return pruntime.Result{}, err
	}
	if err := os.WriteFile(path, []byte(r.content), 0o644); err != nil {
		return pruntime.Result{}, err
	}
	return pruntime.Result{Status: "success", RunID: "run-2"}, nil
}

func writeEvidenceFile(t *testing.T, root string) {
	t.Helper()
	writeFileContent(t, root, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
}

func validTechnicalHandbook() string {
	return `# Sprint Technical Handbook

## Selected Studies And Reports

| Study / Report | Path | Relevant Finding |
| --- | --- | --- |
| 01-project-structure | .ultra/studies/go-cli-study/reports/final/01-project-structure.md | Thin entrypoints are common. |

## Relevant Patterns

- Module-owned behavior with thin CLI wiring.

## Trade-Offs

| Trade-Off | Benefit | Cost |
| --- | --- | --- |
| Local sprint validation | Keeps ownership clear | Some local Markdown checks |

## Anti-Patterns And Warnings

- Do not read unselected evidence opportunistically.

## Open Questions For Reasoning

- How strict should no-decision validation be?

## Evidence Pointers

- .ultra/studies/go-cli-study/reports/final/01-project-structure.md
`
}
