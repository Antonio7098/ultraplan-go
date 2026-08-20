package sprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

func validCodeContext() string {
	return `# Sprint Code Context

## Sprint Scope

Add the selected stage.

## Inspected Repository Areas

- internal/sprint and tests

## Selected Source References

### Canonical stages

- **Path:** ` + "`internal/sprint/domain.go`" + `
- **Lines:** ` + "`30-45`" + `
- **Symbol:** ` + "`PlanningStages`" + `
- **Rationale:** This defines the workflow order.

## Relationships

The service projects the domain state.

## Constraints

The repository is read-only.

## Open Questions

None.
`
}

func TestValidateCodeContextContent(t *testing.T) {
	if findings := ValidateCodeContextContent(validCodeContext()); len(findings) != 0 {
		t.Fatalf("valid findings = %+v", findings)
	}
	for name, mutate := range map[string]func(string) string{
		"empty":       func(string) string { return "  " },
		"preamble":    func(s string) string { return "Here is the requested document:\n\n" + s },
		"placeholder": func(s string) string { return strings.Replace(s, "None.", "TODO", 1) },
		"section":     func(s string) string { return strings.Replace(s, "## Relationships", "## Other", 1) },
		"section body": func(s string) string {
			return strings.Replace(s, "## Constraints\n\nThe repository is read-only.", "## Constraints\n", 1)
		},
		"path": func(s string) string { return strings.Replace(s, "internal/sprint/domain.go", "../secret", 1) },
		"absolute path": func(s string) string {
			return strings.Replace(s, "internal/sprint/domain.go", "/etc/passwd", 1)
		},
		"windows absolute path": func(s string) string {
			return strings.Replace(s, "internal/sprint/domain.go", `C:\\secret.go`, 1)
		},
		"range": func(s string) string { return strings.Replace(s, "30-45", "45-30", 1) },
		"rationale": func(s string) string {
			return strings.Replace(s, "- **Rationale:** This defines the workflow order.\n", "", 1)
		},
		"missing lines": func(s string) string {
			return strings.Replace(s, "- **Lines:** `30-45`\n", "", 1)
		},
		"embedded source": func(s string) string {
			return strings.Replace(s, "## Relationships", "```go\npackage sprint\n```\n\n## Relationships", 1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if findings := ValidateCodeContextContent(mutate(validCodeContext())); len(findings) == 0 {
				t.Fatal("expected actionable finding")
			}
		})
	}
	optionalFields := strings.Replace(validCodeContext(), "- **Symbol:** `PlanningStages`\n", "", 1)
	if findings := ValidateCodeContextContent(optionalFields); len(findings) != 0 {
		t.Fatalf("optional fields findings = %+v", findings)
	}
	multipleRanges := strings.Replace(validCodeContext(), "30-45", "30-45, 50, 61-72", 1)
	if findings := ValidateCodeContextContent(multipleRanges); len(findings) != 0 {
		t.Fatalf("multiple line ranges findings = %+v", findings)
	}
	whitespace := strings.Replace(validCodeContext(), "## Sprint Scope", "  ## Sprint Scope  ", 1)
	if findings := ValidateCodeContextContent(whitespace); len(findings) != 0 {
		t.Fatalf("whitespace findings = %+v", findings)
	}
	template, ok := workspace.DefaultOverrideFile("templates/code-context.md")
	if !ok || len(ValidateCodeContextContent(template)) == 0 {
		t.Fatal("unmodified embedded template must be rejected as placeholder content")
	}
	extra := strings.Repeat("\n### Extra\n\n- **Path:** `internal/x.go`\n- **Lines:** `1-2`\n- **Rationale:** More exact context.\n", 700)
	many := strings.Replace(validCodeContext(), "\n## Relationships", extra+"\n## Relationships", 1)
	findings := ValidateCodeContextContent(many)
	if len(findings) == 0 || findings[0].Problem != "code context exceeds the output budget" {
		t.Fatalf("oversized excerpts findings = %+v", findings)
	}
}

type codeContextRuntime struct {
	request  pruntime.Request
	requests []pruntime.Request
	output   string
	err      error
	result   pruntime.Result
	results  []pruntime.Result
	calls    int
}

type gatedCodeContextRuntime struct {
	started chan struct{}
	release chan struct{}
}

func (r *gatedCodeContextRuntime) StartRun(ctx context.Context, _ pruntime.Request) (pruntime.Result, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return pruntime.Result{}, ctx.Err()
	case <-r.release:
		return pruntime.Result{Status: "completed", TerminalOutput: validCodeContext()}, nil
	}
}

func (r *codeContextRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.calls++
	r.request = req
	r.requests = append(r.requests, req)
	if len(r.results) >= r.calls {
		return r.results[r.calls-1], nil
	}
	result := r.result
	if result.Status == "" {
		result.Status = "success"
	}
	result.TerminalOutput = r.output
	return result, r.err
}

func TestCodeContextArtifactWithoutSuccessfulOutcomeIsNotSkipped(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	writeFileContent(t, sp.Path, validCodeContext(), "code-context.md")
	rt := &codeContextRuntime{output: validCodeContext()}
	result, err := NewService(root).WithRuntime(rt).Flow(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext})
	if err != nil || rt.calls != 1 || result.Message != "code-context complete" {
		t.Fatalf("result=%+v calls=%d err=%v", result, rt.calls, err)
	}
}

func TestCodeContextReadinessRequiresValidNonfailedRequirements(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, "# Requirements\n", "requirements.md")
	status, err := NewService(root).WithoutStatusWrites().Status("proj", "01")
	if err != nil || status.Stages[0].Status != StatusReady || status.Stages[1].Status != StatusMissing {
		t.Fatalf("invalid requirements readiness=%+v err=%v", status.Stages, err)
	}
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	status, err = NewService(root).WithoutStatusWrites().Status("proj", "01")
	if err != nil || status.Stages[0].Status != StatusComplete || status.Stages[1].Status != StatusReady {
		t.Fatalf("valid requirements readiness=%+v err=%v", status.Stages, err)
	}
	now := time.Now().UTC()
	failed := emptyPlanningStageStates(sp)
	failed[0].Status, failed[0].LastRunAt, failed[0].Error = StatusFailed, &now, "requirements generation failed"
	if err := SaveFlowState(root, sp, NewFlowState(sp, failed, now)); err != nil {
		t.Fatal(err)
	}
	status, err = NewService(root).WithoutStatusWrites().Status("proj", "01")
	if err != nil || status.Stages[0].Status != StatusFailed || status.Stages[1].Status != StatusMissing {
		t.Fatalf("failed requirements readiness=%+v err=%v", status.Stages, err)
	}
}

func TestCodeContextPromptDryRunExecutionAndRerunPreservation(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")

	rt := &codeContextRuntime{output: validCodeContext()}
	service := NewService(root).WithRuntime(rt)
	preview, err := service.PromptCodeContext("proj", "01")
	if err != nil || !strings.Contains(preview.Prompt, "Return only the complete `code-context.md`") || !strings.Contains(preview.Prompt, "at or below 65536 bytes") || !strings.Contains(preview.Prompt, "Store references only") || !strings.Contains(preview.Prompt, "Do not copy source text") {
		t.Fatalf("preview err=%v prompt=%s", err, preview.Prompt)
	}
	dry, err := service.FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext, DryRun: true})
	if err != nil || !dry.DryRun || rt.request.Prompt != "" {
		t.Fatalf("dry run=%+v err=%v request=%+v", dry, err, rt.request)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "code-context.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run mutated artifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "flow-state.json")); !os.IsNotExist(err) {
		t.Fatalf("preview or dry-run mutated flow state: %v", err)
	}

	result, err := service.FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext, ModelOverride: "vendor/context", VariantOverride: "max"})
	if err != nil || result.Stages[1].Status != StatusComplete || result.Stages[2].Status != StatusReady {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if rt.request.WorkDir != ApprovedExecuteTargetPath || rt.request.Sandbox != "read_only" || rt.request.Policy.Default != "deny" || rt.request.Policy.Tools["read"] != "allow" || !containsString(rt.request.RequireCaps, "permissions") || rt.request.Provider != "vendor" || rt.request.Model != "context" || rt.request.Metadata["variant"] != "max" || rt.request.Validation != nil {
		t.Fatalf("unsafe or incorrect request: %+v", rt.request)
	}
	if rt.request.PromptRef.ID != "sprint.code-context" || rt.request.PromptRef.Version != "1" || rt.request.PromptRef.OwnerID != "proj/01-alpha" || rt.request.PromptRef.Checksum == "" || rt.request.Metadata["prompt_checksum"] != rt.request.PromptRef.Checksum {
		t.Fatalf("prompt identity not propagated: request=%+v metadata=%+v", rt.request.PromptRef, rt.request.Metadata)
	}
	before, _ := os.ReadFile(filepath.Join(sp.Path, "code-context.md"))
	rt.output = strings.Replace(validCodeContext(), "internal/sprint/domain.go", "../../escape", 1)
	failed, err := service.FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext})
	if err == nil || len(failed.Findings) == 0 || failed.Stages[1].Status != StatusFailed {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
	after, _ := os.ReadFile(filepath.Join(sp.Path, "code-context.md"))
	if string(after) != string(before) {
		t.Fatal("failed rerun replaced the last valid artifact")
	}
}

func TestCodeContextRepairsInvalidTerminalOutputWithinRuntimeBoundary(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")

	runtime := &codeContextRuntime{results: []pruntime.Result{
		{RunID: "initial", SessionID: "session-1", Status: "completed", TerminalOutput: "I will provide the document next."},
		{RunID: "repair", SessionID: "session-1", Status: "completed", TerminalOutput: validCodeContext()},
	}}
	result, err := NewService(root).WithRuntime(runtime).FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext})
	if err != nil || result.Message != "code-context complete" || len(runtime.requests) != 2 {
		t.Fatalf("result=%+v requests=%d err=%v", result, len(runtime.requests), err)
	}
	if runtime.requests[1].SessionID != "session-1" || runtime.requests[1].SessionAction != "continue" {
		t.Fatalf("repair did not continue the original session: %+v", runtime.requests[1])
	}
	for _, want := range []string{"Return only one complete Markdown document", "Do not include a preamble", "Original stage request and context", "missing required section"} {
		if !strings.Contains(runtime.requests[1].Prompt, want) {
			t.Fatalf("repair prompt missing %q: %s", want, runtime.requests[1].Prompt)
		}
	}
	if !result.Runtime.Validation.Configured || !result.Runtime.Validation.Passed || result.Runtime.Repair.AttemptCount != 1 {
		t.Fatalf("runtime validation/repair summary = %+v %+v", result.Runtime.Validation, result.Runtime.Repair)
	}
	data, readErr := os.ReadFile(filepath.Join(sp.Path, "code-context.md"))
	if readErr != nil || string(data) != validCodeContext() {
		t.Fatalf("promoted repaired artifact err=%v content=%q", readErr, string(data))
	}
}

func TestCodeContextUsesRetainedRuntimeEventOutput(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	rt := &codeContextRuntime{result: pruntime.Result{Status: "completed", Events: []pruntime.Event{{Payload: map[string]any{"content": validCodeContext()}}}}}
	result, err := NewService(root).WithRuntime(rt).FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext})
	if err != nil || result.Stages[1].Status != StatusComplete {
		t.Fatalf("event-backed result=%+v err=%v", result, err)
	}
}

func TestCodeContextExecutionLeavesResolvedRepositoryAndUnrelatedArtifactsUnchanged(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	writeFileContent(t, sp.Path, "keep me\n", "unrelated.md")
	target := t.TempDir()
	writeFileContent(t, target, "package source\n", "internal", "source.go")
	writeFileContent(t, target, "package source\n", "internal", "source_test.go")
	writeFileContent(t, target, "ref: refs/heads/main\n", ".git", "HEAD")
	before := snapshotCodeContextTree(t, target)
	requirementsBefore, _ := os.ReadFile(filepath.Join(sp.Path, "requirements.md"))
	unrelatedBefore, _ := os.ReadFile(filepath.Join(sp.Path, "unrelated.md"))
	rt := &codeContextRuntime{output: validCodeContext()}
	service := NewService(root).WithRuntime(rt)
	service.codeContextTarget = func(string) (ExecuteTargetRef, []ValidationFinding) {
		return ExecuteTargetRef{Path: target, Source: "test project index"}, nil
	}
	if _, err := service.FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext}); err != nil {
		t.Fatal(err)
	}
	if rt.request.WorkDir != target || rt.request.Sandbox != "read_only" || rt.request.Permissions != "restricted" {
		t.Fatalf("runtime repository boundary=%+v", rt.request)
	}
	after := snapshotCodeContextTree(t, target)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("resolved repository changed: before=%v after=%v", before, after)
	}
	requirementsAfter, _ := os.ReadFile(filepath.Join(sp.Path, "requirements.md"))
	unrelatedAfter, _ := os.ReadFile(filepath.Join(sp.Path, "unrelated.md"))
	if string(requirementsAfter) != string(requirementsBefore) || string(unrelatedAfter) != string(unrelatedBefore) {
		t.Fatal("execution changed governed input or unrelated sprint artifact")
	}
}

func TestCodeContextFlowUsesSprintMutationConflictBoundary(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	rt := &gatedCodeContextRuntime{started: make(chan struct{}, 1), release: make(chan struct{})}
	service := NewService(root).WithRuntime(rt)
	done := make(chan error, 1)
	go func() {
		_, err := service.FlowStage(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext})
		done <- err
	}()
	<-rt.started
	if _, err := service.FlowStage(context.Background(), "proj", "01-alpha", FlowRequest{To: StageCodeContext}); !errors.Is(err, ErrVerificationConflict) {
		t.Fatalf("conflict error=%v", err)
	}
	close(rt.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func snapshotCodeContextTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestPreCodeContextStateLoadsWithoutStatusWriteAndSerializesOnMutation(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, "# Requirements\n", "requirements.md")
	legacyStages := []StageState{
		{Stage: StageRequirements, Status: StatusComplete, Path: ArtifactRelPath(sp, StageRequirements)},
		{Stage: StageSprintIndex, Status: StatusReady, Path: ArtifactRelPath(sp, StageSprintIndex)},
		{Stage: StageTechnicalHandbook, Status: StatusMissing, Path: ArtifactRelPath(sp, StageTechnicalHandbook)},
		{Stage: StageAreaReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageAreaReasoning)},
		{Stage: StageReasoning, Status: StatusMissing, Path: ArtifactRelPath(sp, StageReasoning)},
		{Stage: StagePlan, Status: StatusMissing, Path: ArtifactRelPath(sp, StagePlan)},
	}
	legacy := FlowState{SchemaVersion: FlowStateSchemaVersion, Project: sp.Project, Sprint: sp.Slug, UpdatedAt: time.Now().UTC(), Stages: legacyStages}
	path, _ := FlowStatePath(root, sp)
	writeJSON(t, path, legacy)
	before, _ := os.ReadFile(path)
	infoBefore, _ := os.Stat(path)
	status, err := NewService(root).Status("proj", "01")
	if err != nil || len(status.Stages) != len(PlanningStages()) || status.Stages[1].Status != StatusSkipped {
		t.Fatalf("status=%+v err=%v", status, err)
	}
	after, _ := os.ReadFile(path)
	infoAfter, _ := os.Stat(path)
	if string(after) != string(before) || !infoAfter.ModTime().Equal(infoBefore.ModTime()) {
		t.Fatal("status rewrote compatible pre-code-context state")
	}
	loaded, err := LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveFlowState(root, sp, loaded); err != nil {
		t.Fatal(err)
	}
	written, _ := os.ReadFile(path)
	if string(written) == string(before) || !strings.Contains(string(written), `"stage": "code-context"`) {
		t.Fatal("later mutation did not serialize canonical state")
	}
}

func TestCodeContextRuntimeFailureDoesNotCreateArtifact(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	rt := &codeContextRuntime{err: errors.New("provider failed")}
	_, err := NewService(root).WithRuntime(rt).FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext})
	if err == nil {
		t.Fatal("expected runtime failure")
	}
	if _, statErr := os.Stat(filepath.Join(sp.Path, "code-context.md")); !os.IsNotExist(statErr) {
		t.Fatalf("runtime failure created authoritative artifact: %v", statErr)
	}
	if matches, _ := filepath.Glob(filepath.Join(sp.Path, ".code-context.*.candidate.md")); len(matches) != 0 {
		t.Fatalf("runtime failure left candidates: %v", matches)
	}
}

func TestCodeContextMissingOutputUnsupportedPermissionsAndCancellationFailClosed(t *testing.T) {
	tests := map[string]struct {
		configure func(*codeContextRuntime) context.Context
		outcome   string
	}{
		"missing output": {configure: func(rt *codeContextRuntime) context.Context { return context.Background() }, outcome: "failed"},
		"timeout": {configure: func(rt *codeContextRuntime) context.Context {
			rt.err = context.DeadlineExceeded
			return context.Background()
		}, outcome: "failed"},
		"unsupported permissions": {configure: func(rt *codeContextRuntime) context.Context {
			rt.output = validCodeContext()
			rt.result.Permissions.UnsupportedCount = 1
			return context.Background()
		}, outcome: "failed"},
		"cancelled": {configure: func(rt *codeContextRuntime) context.Context {
			rt.output = validCodeContext()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, outcome: "cancelled"},
		"interrupted": {configure: func(rt *codeContextRuntime) context.Context {
			rt.result.Status = "interrupted"
			return context.Background()
		}, outcome: "interrupted"},
		"cleanup uncertain": {configure: func(rt *codeContextRuntime) context.Context {
			rt.result.Cleanup = pruntime.CleanupSummary{Attempted: true, Failed: true}
			return context.Background()
		}, outcome: "cleanup_uncertain"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			root := workspaceFixture(t)
			sp := sprintFixture(t, root, "proj", "01-alpha")
			writeFixtureProjectIndex(t, root, "proj")
			writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
			rt := &codeContextRuntime{}
			ctx := test.configure(rt)
			result, err := NewService(root).WithRuntime(rt).FlowCodeContext(ctx, "proj", "01", FlowRequest{To: StageCodeContext})
			if err == nil {
				t.Fatal("expected failure")
			}
			if result.Stages[1].LatestOutcome != test.outcome {
				t.Fatalf("latest outcome=%q want=%q stages=%+v", result.Stages[1].LatestOutcome, test.outcome, result.Stages)
			}
			state, stateErr := LoadFlowState(root, sp)
			if stateErr != nil || state.Stages[1].LatestOutcome != test.outcome {
				t.Fatalf("persisted latest outcome=%q want=%q err=%v", state.Stages[1].LatestOutcome, test.outcome, stateErr)
			}
			if _, err := os.Stat(filepath.Join(sp.Path, "code-context.md")); !os.IsNotExist(err) {
				t.Fatalf("failure created authoritative artifact: %v", err)
			}
		})
	}
}

func TestCodeContextStatePersistenceFailureRestoresPriorArtifact(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, sp.Path, validRequirements("proj", "01-alpha"), "requirements.md")
	old := strings.Replace(validCodeContext(), "Canonical stages", "Prior canonical stages", 1)
	writeFileContent(t, sp.Path, old, "code-context.md")
	statePath, _ := FlowStatePath(root, sp)
	writeFileContent(t, sp.Path, `{"schemaVersion":2,"unexpected":true}`, "flow-state.json")
	rt := &codeContextRuntime{output: validCodeContext()}
	if _, err := NewService(root).WithRuntime(rt).FlowCodeContext(context.Background(), "proj", "01", FlowRequest{To: StageCodeContext}); err == nil {
		t.Fatal("expected state persistence failure")
	}
	got, err := os.ReadFile(filepath.Join(sp.Path, "code-context.md"))
	if err != nil || string(got) != old {
		t.Fatalf("prior artifact not restored: err=%v content=%q", err, got)
	}
	if state, err := os.ReadFile(statePath); err != nil || !strings.Contains(string(state), `"unexpected":true`) {
		t.Fatalf("malformed prior state unexpectedly replaced: err=%v state=%s", err, state)
	}
}

func writeCompletedCodeContext(t *testing.T, root string, sp Sprint) {
	t.Helper()
	writeFileContent(t, sp.Path, validCodeContext(), "code-context.md")
	now := time.Now().UTC()
	if err := SaveFlowState(root, sp, NewFlowState(sp, flowCodeContextSuccessStages(sp, now), now)); err != nil {
		t.Fatal(err)
	}
}
