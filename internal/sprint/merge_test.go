package sprint

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestInspectMergeAcceptsAndFingerprintsDirtySprintWorktree(t *testing.T) {
	source := gitFixture(t)
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "41-merge")
	writeFileContent(t, filepath.Join(root, "projects", "proj"), projectIndexForTarget(source), "project-index.md")
	target, findings := NewService(root).resolveSprintTarget(sp, projectIndexForTarget(source), true)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	if err := os.WriteFile(filepath.Join(target.Path, "README.md"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target.Path, "new.txt"), []byte("untracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	inspection, err := NewService(root).InspectMerge("proj", "41")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(inspection.SourceDirtyPaths, ","); got != "README.md,new.txt" {
		t.Fatalf("dirty paths = %q", got)
	}
	if inspection.SourceWorktreeFingerprint == "" {
		t.Fatal("missing source worktree fingerprint")
	}
	if strings.Contains(strings.Join(inspection.Diagnostics, " "), "sprint worktree is not clean") {
		t.Fatalf("dirty sprint was rejected: %+v", inspection.Diagnostics)
	}
	if got := strings.Join(inspection.ChangedPaths, ","); got != "README.md,new.txt" {
		t.Fatalf("changed paths = %q", got)
	}
}

func TestFlowMergeRequestContinuesOnlyMatchingActiveMerge(t *testing.T) {
	source := gitFixture(t)
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "41-merge")
	target, findings := NewService(root).resolveSprintTarget(sp, projectIndexForTarget(source), true)
	if len(findings) != 0 {
		t.Fatalf("findings = %+v", findings)
	}
	writeFileContent(t, target.Path, "sprint\n", "merged.txt")
	runGitTest(t, target.Path, "add", "merged.txt")
	runGitTest(t, target.Path, "commit", "-m", "sprint change")
	sourceCommit := mustGitOutput(t, target.Path, "rev-parse", "HEAD")
	service := NewService(root)
	state := MergeState{SchemaVersion: mergeStateSchemaVersion, Project: sp.Project, Sprint: sp.Slug, Status: MergeFailed, SourceCommit: sourceCommit}
	if err := service.saveMergeState(sp, state); err != nil {
		t.Fatal(err)
	}
	if got := service.mergeRequestForFlow("proj", "41", MergeRequest{Confirm: true}); got.Continue {
		t.Fatal("flow continued without an active merge")
	}
	runGitTest(t, source, "merge", "--no-ff", "--no-commit", sourceCommit)
	if got := service.mergeRequestForFlow("proj", "41", MergeRequest{Confirm: true}); !got.Continue {
		t.Fatal("flow did not continue the matching active merge")
	}
	state.SourceCommit = strings.Repeat("0", 40)
	if err := service.saveMergeState(sp, state); err != nil {
		t.Fatal(err)
	}
	if got := service.mergeRequestForFlow("proj", "41", MergeRequest{Confirm: true}); got.Continue {
		t.Fatal("flow continued a stale active merge")
	}
}

func TestCommitSprintSnapshotCapturesTrackedAndUntrackedChanges(t *testing.T) {
	source := gitFixture(t)
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := mustGitOutput(t, source, "rev-parse", "HEAD")
	description := MergeDescription{Title: "Capture sprint work", Summary: []string{"Records the completed implementation."}}
	if err := commitSprintSnapshot(source, Sprint{Project: "proj", Slug: "41-merge"}, description); err != nil {
		t.Fatal(err)
	}
	after := mustGitOutput(t, source, "rev-parse", "HEAD")
	if before == after {
		t.Fatal("snapshot did not create a commit")
	}
	if status := mustGitOutput(t, source, "status", "--porcelain"); status != "" {
		t.Fatalf("status = %q", status)
	}
	if paths := mustGitOutput(t, source, "show", "--format=", "--name-only", "HEAD"); !strings.Contains(paths, "README.md") || !strings.Contains(paths, "new.txt") {
		t.Fatalf("snapshot paths = %q", paths)
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

func TestDecodeMergeDescriptionFromNestedMarkdownWithScalarLists(t *testing.T) {
	run := pruntime.Result{Events: []pruntime.Event{{Payload: map[string]any{
		"message": map[string]any{"part": map[string]any{"text": "reasoning {not json}\n```json\n{\"title\":\"Add bounded QA repair\",\"summary\":\"Adds the repair lifecycle.\",\"verification\":\"go test ./...\",\"risk_notes\":[\"Schema readers fail closed.\"]}\n```"}},
	}}}}
	var description MergeDescription
	if err := decodeRuntimeJSON(run, &description); err != nil {
		t.Fatal(err)
	}
	if err := validateMergeDescription(description); err != nil {
		t.Fatal(err)
	}
	if got := description.Summary; len(got) != 1 || got[0] != "Adds the repair lifecycle." {
		t.Fatalf("summary = %#v", got)
	}
	if got := description.Verification; len(got) != 1 || got[0] != "go test ./..." {
		t.Fatalf("verification = %#v", got)
	}
}

func TestDecodeMergeDescriptionSplitsLongScalarEntries(t *testing.T) {
	longSummary := strings.Repeat("bounded repair work ", 30)
	run := pruntime.Result{TerminalOutput: `{"title":"Add bounded repair","summary":` + strconv.Quote(longSummary) + `}`}
	var description MergeDescription
	if err := decodeRuntimeJSON(run, &description); err != nil {
		t.Fatal(err)
	}
	if len(description.Summary) < 2 {
		t.Fatalf("summary was not split: %#v", description.Summary)
	}
	if err := validateMergeDescription(description); err != nil {
		t.Fatal(err)
	}
}

func TestValidateMergeDescriptionRejectsUnsafeTitle(t *testing.T) {
	err := validateMergeDescription(MergeDescription{Title: "bad\ntitle", Summary: []string{"summary"}})
	if err == nil {
		t.Fatal("expected invalid title")
	}
}
