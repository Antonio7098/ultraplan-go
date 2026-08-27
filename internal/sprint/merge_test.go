package sprint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestSprintWorkspaceRecordsIntegrationBranch(t *testing.T) {
	source := gitFixture(t)
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "41-merge")
	_, findings := NewService(root).resolveSprintTarget(sp, projectIndexForTarget(source), true)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	record, err := loadSprintWorkspace(sp)
	if err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != 2 || record.IntegrationBranch != "main" {
		t.Fatalf("record = %+v", record)
	}
}

func TestInspectMergeReportsDeterministicCommitAndVerificationGate(t *testing.T) {
	source := gitFixture(t)
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "41-merge")
	writeFileContent(t, filepath.Join(root, "projects", "proj"), projectIndexForTarget(source), "project-index.md")
	target, findings := NewService(root).resolveSprintTarget(sp, projectIndexForTarget(source), true)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	if err := os.WriteFile(filepath.Join(target.Path, "merged.txt"), []byte("sprint\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, target.Path, "add", "merged.txt")
	runGitTest(t, target.Path, "commit", "-m", "sprint change")

	inspection, err := NewService(root).InspectMerge("proj", "41")
	if err != nil {
		t.Fatal(err)
	}
	if inspection.SourceBranch != "ultraplan/proj/41-merge" || inspection.TargetBranch != "main" || inspection.SourceCommit == inspection.Baseline {
		t.Fatalf("inspection = %+v", inspection)
	}
	if len(inspection.ChangedPaths) != 1 || inspection.ChangedPaths[0] != "merged.txt" {
		t.Fatalf("changed paths = %v", inspection.ChangedPaths)
	}
	if inspection.Ready || !strings.Contains(strings.Join(inspection.Diagnostics, " "), "verification") {
		t.Fatalf("inspection = %+v", inspection)
	}
}

func TestDecodeAndValidateMergeDescription(t *testing.T) {
	run := pruntime.Result{TerminalOutput: "result:\n```json\n{\"title\":\"Merge sprint work\",\"summary\":[\"Adds governed integration\"],\"verification\":[\"go test ./...\"]}\n```"}
	var description MergeDescription
	if err := decodeRuntimeJSON(run, &description); err != nil {
		t.Fatal(err)
	}
	if err := validateMergeDescription(description); err != nil {
		t.Fatal(err)
	}
	if got := renderMergeCommitMessage(description); !strings.HasPrefix(got, "Merge sprint work\n\n- Adds") {
		t.Fatalf("message = %q", got)
	}
}

func TestValidateMergeDescriptionRejectsUnsafeTitle(t *testing.T) {
	err := validateMergeDescription(MergeDescription{Title: "bad\ntitle", Summary: []string{"summary"}})
	if err == nil {
		t.Fatal("expected invalid title")
	}
}
