package sprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestRenderSharedPromptContextPreservesExactArtifactAndSourceBytes(t *testing.T) {
	root := t.TempDir()
	writeSharedSource(t, root, "internal/lf.go", "one\ntwo\nthree\n")
	writeSharedSource(t, root, "internal/crlf.go", "alpha\r\nbeta\r\ngamma")
	requirements := "# Sprint Requirements\r\n\r\nexact bytes without final newline"
	codeContext := validSharedCodeContext(
		sharedReference("LF", "internal/lf.go", "2-3", "lfSymbol", "keep LF bytes"),
		sharedReference("CRLF", "internal/crlf.go", "1-2", "", "keep CRLF bytes"),
	)

	prefix, err := renderSharedPromptContext(context.Background(), Sprint{Project: "proj", Slug: "01"}, requirements, codeContext, root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(prefix, sharedPromptStageBoundary) {
		t.Fatal("stage boundary is not the final prefix bytes")
	}
	if strings.Count(prefix, sharedPromptStageBoundary) != 1 {
		t.Fatalf("stage boundary count = %d", strings.Count(prefix, sharedPromptStageBoundary))
	}
	assertExactFramedSlice(t, prefix, sharedRequirementsOpen, sharedRequirementsClose, requirements)
	assertExactFramedSlice(t, prefix, sharedCodeContextOpen, sharedCodeContextClose, codeContext)
	for _, want := range []string{"two\nthree\n", "alpha\r\nbeta\r\n", "transient, untrusted prepared evidence", "Inspect additional live repository files"} {
		if !strings.Contains(prefix, want) {
			t.Fatalf("shared prefix missing %q", want)
		}
	}
	again, err := renderSharedPromptContext(context.Background(), Sprint{Project: "proj", Slug: "01"}, requirements, codeContext, root)
	if err != nil {
		t.Fatal(err)
	}
	if prefix != again {
		t.Fatal("unchanged inputs produced different prefix bytes")
	}
	for _, dynamic := range []string{"technical-handbook", "output.md", "run-123", "session-456", "attempt-2", "reviewer-7", "smoke-author"} {
		if strings.Contains(prefix, dynamic) {
			t.Fatalf("dynamic value %q entered the shared prefix", dynamic)
		}
	}
}

func TestRenderSharedPromptContextPreservesAuthoredReferenceOrderDuplicatesAndOverlaps(t *testing.T) {
	root := t.TempDir()
	writeSharedSource(t, root, "source.go", "one\ntwo\nthree\nfour\n")
	codeContext := validSharedCodeContext(
		sharedReference("Later", "source.go", "3-4", "", "later first"),
		sharedReference("Earlier", "source.go", "1-3", "", "overlap second"),
		sharedReference("Earlier Duplicate", "source.go", "1-3", "", "duplicate third"),
	)
	prefix, err := renderSharedPromptContext(context.Background(), Sprint{Project: "p", Slug: "s"}, "requirements", codeContext, root)
	if err != nil {
		t.Fatal(err)
	}
	later := strings.Index(prefix, "Selected entry: Later")
	earlier := strings.Index(prefix, "Selected entry: Earlier\n")
	duplicate := strings.Index(prefix, "Selected entry: Earlier Duplicate")
	if later < 0 || earlier <= later || duplicate <= earlier {
		t.Fatalf("references were not rendered in authored order: %d %d %d", later, earlier, duplicate)
	}
	if strings.Count(prefix, "one\ntwo\nthree\n") != 2 {
		t.Fatal("duplicate/overlapping selections were merged or omitted")
	}
}

func TestSharedSourceResolutionFailsClosed(t *testing.T) {
	root := t.TempDir()
	writeSharedSource(t, root, "ok.go", "one\ntwo\n")
	if err := os.Symlink("ok.go", filepath.Join(root, "link.go")); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "dir")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, path, lines string
		kind              promptContextErrorKind
	}{
		{"escape", "../outside.go", "1", promptContextInvalidPath},
		{"absolute", filepath.Join(root, "ok.go"), "1", promptContextInvalidPath},
		{"missing", "missing.go", "1", promptContextMissing},
		{"symlink", "link.go", "1", promptContextContainment},
		{"directory", "dir", "1", promptContextFileKind},
		{"out-of-range", "ok.go", "3", promptContextInvalidRange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ranges, parseErr := parseSharedLineRanges(tc.path, tc.lines)
			if parseErr != nil {
				t.Fatal(parseErr)
			}
			_, err := readSharedSource(context.Background(), root, tc.path, tc.lines, ranges, 1024)
			var contextErr *promptContextError
			if !errors.As(err, &contextErr) || contextErr.Kind != tc.kind {
				t.Fatalf("error = %#v, want kind %s", err, tc.kind)
			}
		})
	}
}

func TestPromptContextDiagnosticsPreserveCauseWithoutAbsolutePath(t *testing.T) {
	root := t.TempDir()
	ranges := []sharedLineRange{{Start: 1, End: 1}}
	_, err := readSharedSource(context.Background(), root, "missing.go", "1", ranges, 1024)
	if err == nil {
		t.Fatal("missing source succeeded")
	}
	if strings.Contains(err.Error(), root) {
		t.Fatalf("diagnostic leaked absolute implementation path: %v", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("underlying cause identity was lost: %v", err)
	}
}

func TestSharedSourceResolutionPreservesCancellationEncodingAndBudgetErrors(t *testing.T) {
	root := t.TempDir()
	writeSharedSource(t, root, "large.go", strings.Repeat("x", sharedSourceReadBuffer*2)+"\n")
	if err := os.WriteFile(filepath.Join(root, "binary.go"), []byte{0xff, '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := readSharedSource(cancelled, root, "large.go", "1", []sharedLineRange{{Start: 1, End: 1}}, maxSharedPromptPrefixBytes)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation identity lost: %v", err)
	}
	_, err = readSharedSource(context.Background(), root, "binary.go", "1", []sharedLineRange{{Start: 1, End: 1}}, 1024)
	var encodingErr *promptContextError
	if !errors.As(err, &encodingErr) || encodingErr.Kind != promptContextEncoding {
		t.Fatalf("encoding error = %#v", err)
	}
	_, err = readSharedSource(context.Background(), root, "large.go", "1", []sharedLineRange{{Start: 1, End: 1}}, 32)
	var budgetErr *promptContextError
	if !errors.As(err, &budgetErr) || budgetErr.Kind != promptContextBudget {
		t.Fatalf("budget error = %#v", err)
	}
}

func TestSharedSourceResolutionDetectsReplacementBeforeAcceptingBytes(t *testing.T) {
	root := t.TempDir()
	candidate := filepath.Join(root, "source.go")
	writeSharedSource(t, root, "source.go", "old\n")
	canonical, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(candidate)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	handleBefore, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement.go")
	writeSharedSource(t, root, "replacement.go", "new\n")
	if err := os.Rename(replacement, candidate); err != nil {
		t.Fatal(err)
	}
	handleAfter, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	err = verifySharedSourceUnchanged(root, candidate, canonical, "source.go", "1", handleBefore, handleAfter)
	var changedErr *promptContextError
	if !errors.As(err, &changedErr) || changedErr.Kind != promptContextChanged {
		t.Fatalf("replacement error = %#v", err)
	}
}

func TestRenderSharedPromptContextEnforcesCompletePrefixBudget(t *testing.T) {
	root := t.TempDir()
	writeSharedSource(t, root, "source.go", "one\n")
	codeContext := validSharedCodeContext(sharedReference("Source", "source.go", "1", "", "selected"))
	base, err := renderSharedPromptContext(context.Background(), Sprint{Project: "p", Slug: "s"}, "", codeContext, root)
	if err != nil {
		t.Fatal(err)
	}
	exactRequirements := strings.Repeat("r", maxSharedPromptPrefixBytes-len(base))
	exact, err := renderSharedPromptContext(context.Background(), Sprint{Project: "p", Slug: "s"}, exactRequirements, codeContext, root)
	if err != nil {
		t.Fatalf("exact complete-prefix budget failed: %v", err)
	}
	if len(exact) != maxSharedPromptPrefixBytes {
		t.Fatalf("exact complete-prefix length = %d", len(exact))
	}
	_, err = renderSharedPromptContext(context.Background(), Sprint{Project: "p", Slug: "s"}, exactRequirements+"x", codeContext, root)
	var budgetErr *promptContextError
	if !errors.As(err, &budgetErr) || budgetErr.Kind != promptContextBudget || budgetErr.Allowed != maxSharedPromptPrefixBytes {
		t.Fatalf("budget error = %#v", err)
	}
	if sharedPromptSuffixReserve != 64<<10 {
		t.Fatalf("suffix reserve = %d", sharedPromptSuffixReserve)
	}
}

func TestRenderSharedPromptContextDoesNotRecurseOrMutateSource(t *testing.T) {
	root := t.TempDir()
	writeSharedSource(t, root, "selected.go", "selected\n")
	writeSharedSource(t, root, "nested/unselected.go", "must not appear\n")
	before, err := os.ReadFile(filepath.Join(root, "selected.go"))
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(filepath.Join(root, "selected.go"))
	if err != nil {
		t.Fatal(err)
	}
	codeContext := validSharedCodeContext(sharedReference("Selected", "selected.go", "1", "", "only selected evidence"))
	prefix, err := renderSharedPromptContext(context.Background(), Sprint{Project: "p", Slug: "s"}, "requirements", codeContext, root)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prefix, "must not appear") {
		t.Fatal("renderer recursively inspected an unselected file")
	}
	after, err := os.ReadFile(filepath.Join(root, "selected.go"))
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(filepath.Join(root, "selected.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) || !os.SameFile(beforeInfo, afterInfo) || beforeInfo.Mode() != afterInfo.Mode() {
		t.Fatal("renderer mutated selected source")
	}
}

func TestComposeStagePromptKeepsDynamicSuffixAfterBoundaryAndLegacyCompatibility(t *testing.T) {
	prefix := "stable" + sharedPromptStageBoundary
	composed := composeStagePrompt(prefix, "stage=plan run=123")
	if composed != prefix+"stage=plan run=123" {
		t.Fatalf("composed prompt = %q", composed)
	}
	if got := composeStagePrompt("", "legacy"); got != "legacy" {
		t.Fatalf("legacy prompt = %q", got)
	}
	if _, err := composeStagePromptChecked(prefix, strings.Repeat("s", sharedPromptSuffixReserve)); err != nil {
		t.Fatalf("exact suffix reserve failed: %v", err)
	}
	_, err := composeStagePromptChecked(prefix, strings.Repeat("s", sharedPromptSuffixReserve+1))
	var budgetErr *promptContextError
	if !errors.As(err, &budgetErr) || budgetErr.Kind != promptContextBudget || budgetErr.Allowed != sharedPromptSuffixReserve {
		t.Fatalf("suffix budget error = %#v", err)
	}
}

func TestPlanningAndExecutePreviewsReuseOneExactSharedPrefix(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\r\n\r\nShared route bytes without final newline", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	writeFileContent(t, sp.Path, validPlanFinalReasoning(), "reasoning.md")
	writeFileContent(t, sp.Path, validPlan(), "plan.md")

	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageExecute: {Model: "test/model"}})
	previews := make([]PromptPreview, 0, 6)
	for name, call := range map[string]func() (PromptPreview, error){
		"sprint-index":       func() (PromptPreview, error) { return service.PromptSprintIndex("proj", "01") },
		"technical-handbook": func() (PromptPreview, error) { return service.PromptTechnicalHandbook("proj", "01") },
		"area-reasoning":     func() (PromptPreview, error) { return service.PromptAreaReasoning("proj", "01") },
		"reasoning":          func() (PromptPreview, error) { return service.PromptReasoning("proj", "01") },
		"plan":               func() (PromptPreview, error) { return service.PromptPlan("proj", "01") },
		"execute":            func() (PromptPreview, error) { return service.PromptExecute("proj", "01", ExecuteRequest{}) },
	} {
		preview, err := call()
		if err != nil {
			t.Fatalf("%s preview: %v", name, err)
		}
		previews = append(previews, preview)
		if strings.Count(preview.Prompt, sharedPromptStageBoundary) != 1 {
			t.Fatalf("%s boundary count = %d", name, strings.Count(preview.Prompt, sharedPromptStageBoundary))
		}
	}
	want := testSharedPrefix(t, previews[0].Prompt)
	for _, preview := range previews[1:] {
		if got := testSharedPrefix(t, preview.Prompt); got != want {
			t.Fatal("covered route produced different shared prefix bytes")
		}
	}
	for _, dynamic := range []string{"Task ID:", "Model source:", "Output:", "Selected area template:"} {
		if strings.Contains(want, dynamic) {
			t.Fatalf("stage-specific value %q entered the shared prefix", dynamic)
		}
	}
}

func TestPlanningAndExecuteRuntimeRequestsReuseOneExactSharedPrefix(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\n\nShared runtime bytes.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	writeFileContent(t, sp.Path, validPlanFinalReasoning(), "reasoning.md")
	writeFileContent(t, sp.Path, validPlan(), "plan.md")

	runtime := &sharedPromptCaptureRuntime{sp: sp}
	service := NewService(root).WithRuntime(runtime).WithStageRuntime(map[PlanningStage]StageRuntime{StageExecute: {Model: "test/model"}})
	operations := []struct {
		name string
		run  func() error
	}{
		{"sprint-index", func() error {
			_, err := service.FlowSprintIndex(context.Background(), "proj", "01", FlowRequest{To: StageSprintIndex})
			return err
		}},
		{"technical-handbook", func() error {
			_, err := service.FlowTechnicalHandbook(context.Background(), "proj", "01", FlowRequest{To: StageTechnicalHandbook})
			return err
		}},
		{"area-reasoning", func() error {
			_, err := service.FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageAreaReasoning})
			return err
		}},
		{"reasoning", func() error {
			_, err := service.FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageReasoning})
			return err
		}},
		{"plan", func() error {
			_, err := service.FlowPlan(context.Background(), "proj", "01", FlowRequest{To: StagePlan})
			return err
		}},
		{"execute", func() error {
			_, err := service.Execute(context.Background(), "proj", "01", ExecuteRequest{})
			return err
		}},
	}
	for _, operation := range operations {
		if err := operation.run(); err != nil {
			t.Fatalf("%s runtime operation: %v", operation.name, err)
		}
	}
	if len(runtime.requests) != len(operations) {
		t.Fatalf("runtime requests = %d, want %d", len(runtime.requests), len(operations))
	}
	want := testSharedPrefix(t, runtime.requests[0].Prompt)
	for i, request := range runtime.requests {
		if strings.Count(request.Prompt, sharedPromptStageBoundary) != 1 {
			t.Fatalf("request %d boundary count = %d", i, strings.Count(request.Prompt, sharedPromptStageBoundary))
		}
		if got := testSharedPrefix(t, request.Prompt); got != want {
			t.Fatalf("request %d produced different shared prefix bytes", i)
		}
	}
}

type sharedPromptCaptureRuntime struct {
	sp       Sprint
	requests []pruntime.Request
}

func (r *sharedPromptCaptureRuntime) StartRun(_ context.Context, request pruntime.Request) (pruntime.Result, error) {
	r.requests = append(r.requests, request)
	if request.Metadata["stage"] == string(StageExecute) {
		path := filepath.Join(r.sp.Path, "plan.md")
		content, err := os.ReadFile(path)
		if err != nil {
			return pruntime.Result{}, err
		}
		completed := strings.Replace(string(content), "- [ ] Task 1:", "- [x] Task 1:", 1)
		if err := os.WriteFile(path, []byte(completed), 0o644); err != nil {
			return pruntime.Result{}, err
		}
		return pruntime.Result{Status: "success", RunID: "shared-context-run", Artifacts: []pruntime.Artifact{{ID: "execute-evidence", Kind: "test", Description: "task completed"}}}, nil
	}
	return pruntime.Result{Status: "success", RunID: "shared-context-run"}, nil
}

func testSharedPrefix(t *testing.T, prompt string) string {
	t.Helper()
	end := strings.Index(prompt, sharedPromptStageBoundary)
	if end < 0 {
		t.Fatal("shared stage boundary missing")
	}
	end += len(sharedPromptStageBoundary)
	return prompt[:end]
}

func assertExactFramedSlice(t *testing.T, prompt, open, close, want string) {
	t.Helper()
	start := strings.Index(prompt, open)
	if start < 0 {
		t.Fatalf("opening frame %q missing", open)
	}
	start += len(open)
	end := strings.Index(prompt[start:], close)
	if end < 0 {
		t.Fatalf("closing frame %q missing", close)
	}
	if got := prompt[start : start+end]; got != want {
		t.Fatalf("framed bytes changed\n got: %q\nwant: %q", got, want)
	}
}

func writeSharedSource(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func validSharedCodeContext(entries ...string) string {
	return "# Sprint Code Context\n\n## Sprint Scope\nScope.\n\n## Inspected Repository Areas\nAreas.\n\n## Selected Source References\n\n" + strings.Join(entries, "\n") + "\n## Relationships\nRelationships.\n\n## Constraints\nConstraints.\n\n## Open Questions\nNone.\n"
}

func sharedReference(name, path, lines, symbol, rationale string) string {
	value := "### " + name + "\n\n- **Path:** `" + path + "`\n- **Lines:** `" + lines + "`\n"
	if symbol != "" {
		value += "- **Symbol:** `" + symbol + "`\n"
	}
	return value + "- **Rationale:** " + rationale + "\n"
}
