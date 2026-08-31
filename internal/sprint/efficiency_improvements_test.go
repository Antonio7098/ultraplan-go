package sprint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func TestContextPackFreezesResolvedEvidenceAcrossLiveSourceChanges(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-context-pack")
	target := t.TempDir()
	writeSharedSource(t, target, "source.go", "original\nsecond\n")
	requirements := "# Requirements\n"
	codeContext := validSharedCodeContext(sharedReference("Source", "source.go", "1-2", "", "selected"))
	prefix, err := renderSharedPromptContext(context.Background(), sp, requirements, codeContext, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveContextPack(root, sp, requirements, codeContext, target, prefix, time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	writeSharedSource(t, target, "source.go", "changed\nsecond\n")
	cached, err := loadContextPack(root, sp, requirements, codeContext, target)
	if err != nil {
		t.Fatal(err)
	}
	if cached != prefix || !strings.Contains(cached, "original\nsecond") || strings.Contains(cached, "changed\nsecond") {
		t.Fatalf("context pack did not preserve original evidence:\n%s", cached)
	}
	live, err := renderSharedPromptContext(context.Background(), sp, requirements, codeContext, target)
	if err != nil {
		t.Fatal(err)
	}
	if live == cached {
		t.Fatal("live renderer unexpectedly matched frozen context after source change")
	}
	if _, err := loadContextPack(root, sp, requirements+"changed", codeContext, target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("changed governed artifact unexpectedly matched old context pack: %v", err)
	}
}

func TestRuntimeCompositionLazilyCachesButPreviewRemainsReadOnly(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-lazy-context-pack")
	target := t.TempDir()
	writeSharedSource(t, target, "source.go", "first\nsecond\n")
	inputs := PlanningInputs{
		Requirements: "# Requirements\n",
		CodeContext:  validSharedCodeContext(sharedReference("Source", "source.go", "1-2", "", "selected")),
	}
	writeFileContent(t, sp.Path, inputs.Requirements, "requirements.md")
	writeFileContent(t, sp.Path, inputs.CodeContext, "code-context.md")
	now := time.Unix(1, 0).UTC()
	if err := SaveFlowState(root, sp, NewFlowState(sp, flowCodeContextSuccessStages(sp, now), now)); err != nil {
		t.Fatal(err)
	}
	service := NewService(root)
	service.codeContextTarget = func(string) (ExecuteTargetRef, []ValidationFinding) {
		return ExecuteTargetRef{Path: target}, nil
	}
	key, _, _, _ := contextPackIdentity(inputs.Requirements, inputs.CodeContext, target)
	packPath := contextPackPath(root, sp, key)
	if _, err := service.prepareSharedPromptContext(context.Background(), sp, inputs, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(packPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read-only preview persisted context pack: %v", err)
	}
	prefix, err := service.prepareSharedPromptContext(context.Background(), sp, inputs, true)
	if err != nil || !strings.Contains(prefix, "first\nsecond") {
		t.Fatalf("runtime prefix err=%v prefix=%q", err, prefix)
	}
	if _, err := os.Stat(packPath); err != nil {
		t.Fatalf("runtime composition did not lazily persist context pack: %v", err)
	}
}

func TestCanonicalSharedSelectionHasNoReferenceOrLineLimit(t *testing.T) {
	refs := make([]sharedContextReference, 74)
	for i := range refs {
		refs[i] = sharedContextReference{Name: "selection", Path: "source.go", Lines: "1", Rationale: "test"}
	}
	selections, err := canonicalSharedSelections(refs)
	if err != nil {
		t.Fatalf("74 references were rejected: %v", err)
	}
	if len(selections) != 1 || len(selections[0].References) != len(refs) {
		t.Fatalf("74 reference selection = %#v", selections)
	}
	selections, err = canonicalSharedSelections([]sharedContextReference{{Name: "selection", Path: "source.go", Lines: "1-4097", Rationale: "test"}})
	if err != nil {
		t.Fatalf("large line range was rejected: %v", err)
	}
	if len(selections) != 1 || selections[0].Lines != "1-4097" {
		t.Fatalf("large line range selection = %#v", selections)
	}
}

func TestPlanAndExecuteInjectCompleteTechnicalHandbook(t *testing.T) {
	for _, heading := range []string{"Examples Worth Investigating", "Examples Worth Inspecting"} {
		t.Run(heading, func(t *testing.T) {
			_, sp, service := executePersistenceFixture(t, &recordingExecuteRuntime{})
			example := "Inspect `internal/sprint/handoff.go` for the bounded handoff pattern."
			handbook := strings.Replace(validReasoningTechnicalHandbook(), "## Open Questions For Reasoning", "## "+heading+"\n\n- "+example+"\n\n## Handbook Background Only\n\nDO-NOT-INJECT\n\n## Open Questions For Reasoning", 1)
			writeFileContent(t, sp.Path, handbook, "technical-handbook.md")

			plan, err := service.PromptPlan("proj", "01")
			if err != nil {
				t.Fatal(err)
			}
			execute, err := service.PromptExecute("proj", "01", ExecuteRequest{})
			if err != nil {
				t.Fatal(err)
			}
			for stage, prompt := range map[string]string{"plan": plan.Prompt, "execute": execute.Prompt} {
				if strings.Count(prompt, example) != 1 || !strings.Contains(prompt, "ID: technical-handbook") || !strings.Contains(prompt, "Mode: full") {
					t.Fatalf("%s prompt did not inject the complete handbook exactly once:\n%s", stage, prompt)
				}
				if !strings.Contains(prompt, "DO-NOT-INJECT") {
					t.Fatalf("%s prompt retained the old selected-section behavior", stage)
				}
			}
		})
	}
	for _, stage := range []PlanningStage{StagePlan, StageExecute} {
		if !strings.Contains(strings.Join(InputContract(stage).Required, ","), "technical-handbook") || len(InputContract(stage).Optional) != 0 {
			t.Fatalf("%s input contract = %+v", stage, InputContract(stage))
		}
	}
}

func TestExecuteUsesOneSessionWithCompactPerTaskContinuations(t *testing.T) {
	runtime := &batchExecutionRuntime{}
	_, sp, service := executePersistenceFixture(t, runtime)
	plan := strings.Replace(validPlan(), "## Evidence Checklist", "- [ ] Task 2: Verify shared execution for Decision 1 / AC-01\n  > Executes: Decision 1, AC-01\n  - [ ] Verification expectation: go test ./...\n\n## Evidence Checklist", 1)
	writeFileContent(t, sp.Path, plan, "plan.md")
	handbook := strings.Replace(validReasoningTechnicalHandbook(), "## Open Questions For Reasoning", "## Examples Worth Investigating\n\n- Inspect the shared execution session.\n\n## Open Questions For Reasoning", 1)
	writeFileContent(t, sp.Path, handbook, "technical-handbook.md")
	_, planTasks, target, selection, findings, err := service.prepareExecute("proj", "01", ExecuteRequest{})
	if err != nil || len(findings) != 0 {
		t.Fatalf("prepare execute findings=%+v err=%v", findings, err)
	}
	inputs, err := service.store.ReadPlanningInputs(sp)
	if err != nil {
		t.Fatal(err)
	}
	prefix, err := service.prepareSharedPromptContext(context.Background(), sp, inputs, false)
	if err != nil {
		t.Fatal(err)
	}
	independentPromptBytes := 0
	for _, task := range planTasks {
		prompt, composeErr := composeStagePromptChecked(prefix, service.renderExecuteSessionPrompt(sp, task, planTasks, target, selection))
		if composeErr != nil {
			t.Fatal(composeErr)
		}
		independentPromptBytes += len(prompt)
	}

	result, err := service.Execute(context.Background(), "proj", "01", ExecuteRequest{})
	if err != nil {
		t.Fatalf("execute result=%+v err=%v", result, err)
	}
	if len(runtime.requests) != 2 || len(result.Tasks) != 2 {
		t.Fatalf("requests=%d tasks=%+v", len(runtime.requests), result.Tasks)
	}
	first, second := runtime.requests[0], runtime.requests[1]
	if first.SessionID != "" || first.SessionAction != "" || first.Metadata["execution_session_mode"] != "initial" || first.Metadata["execution_queue_size"] != "2" {
		t.Fatalf("initial execution request = %+v", first)
	}
	if !strings.Contains(first.Prompt, sharedPromptStageBoundary) || !strings.Contains(first.Prompt, "Inspect the shared execution session") || !strings.Contains(first.Prompt, "Task 1:") || !strings.Contains(first.Prompt, "Task 2:") {
		t.Fatalf("initial execution primer missing shared context, examples, or queue:\n%s", first.Prompt)
	}
	if second.SessionID != "execute-shared-session" || second.SessionAction != "continue" || second.Metadata["execution_session_mode"] != "continue" || second.Metadata["execution_turn"] != "2" {
		t.Fatalf("continuation execution request = %+v", second)
	}
	if strings.Contains(second.Prompt, sharedPromptStageBoundary) || strings.Contains(second.Prompt, "Inspect the shared execution session") || !strings.Contains(second.Prompt, "# Continue Sprint Execution") || !strings.Contains(second.Prompt, "Task 2:") {
		t.Fatalf("continuation was not a compact task delta:\n%s", second.Prompt)
	}
	sharedSessionPromptBytes := len(first.Prompt) + len(second.Prompt)
	if sharedSessionPromptBytes >= independentPromptBytes {
		t.Fatalf("shared session prompts=%d independent prompts=%d", sharedSessionPromptBytes, independentPromptBytes)
	}
	t.Logf("metric execute_session.independent_prompt_bytes=%d", independentPromptBytes)
	t.Logf("metric execute_session.shared_prompt_bytes=%d", sharedSessionPromptBytes)
	for _, task := range result.Tasks {
		if task.Status != ExecuteTaskComplete || task.Runtime == nil || task.Runtime.SessionID != "execute-shared-session" {
			t.Fatalf("task did not retain shared execution session: %+v", task)
		}
	}
}

func TestExecuteFallsBackToFullPromptsWithoutReusableRuntimeSession(t *testing.T) {
	runtime := &noSessionExecutionRuntime{}
	_, sp, service := executePersistenceFixture(t, runtime)
	plan := strings.Replace(validPlan(), "## Evidence Checklist", "- [ ] Task 2: Verify fallback for Decision 1 / AC-01\n  > Executes: Decision 1, AC-01\n  - [ ] Verification expectation: go test ./...\n\n## Evidence Checklist", 1)
	writeFileContent(t, sp.Path, plan, "plan.md")
	result, err := service.Execute(context.Background(), "proj", "01", ExecuteRequest{})
	if err != nil {
		t.Fatalf("execute result=%+v err=%v", result, err)
	}
	if len(runtime.requests) != 2 || runtime.requests[1].SessionID != "" || runtime.requests[1].SessionAction != "" || runtime.requests[1].Metadata["execution_session_mode"] != "fresh-fallback" || !strings.Contains(runtime.requests[1].Prompt, sharedPromptStageBoundary) {
		t.Fatalf("fresh fallback requests = %+v", runtime.requests)
	}
}

func TestExecuteStopsSharedQueueAfterTaskFailure(t *testing.T) {
	runtime := &failingBatchExecutionRuntime{}
	_, sp, service := executePersistenceFixture(t, runtime)
	plan := strings.Replace(validPlan(), "## Evidence Checklist", "- [ ] Task 2: Must not run after failure for Decision 1 / AC-01\n  > Executes: Decision 1, AC-01\n  - [ ] Verification expectation: go test ./...\n\n## Evidence Checklist", 1)
	writeFileContent(t, sp.Path, plan, "plan.md")
	result, err := service.Execute(context.Background(), "proj", "01", ExecuteRequest{})
	if err == nil || len(runtime.requests) != 1 || len(result.Tasks) != 2 || result.Tasks[0].Status != ExecuteTaskFailed || result.Tasks[1].Status != ExecuteTaskPending {
		t.Fatalf("failed shared queue requests=%d result=%+v err=%v", len(runtime.requests), result, err)
	}
}

func TestRuntimeRequestCarriesStableCacheDirective(t *testing.T) {
	service := NewService(t.TempDir())
	prompt := "stable" + sharedPromptStageBoundary + "volatile"
	req := service.runtimeRequest(prompt, map[string]string{"project": "proj", "sprint": "01", "stage": string(StagePlan)})
	if req.Cache.Key == "" || req.Cache.BreakpointBytes != len("stable"+sharedPromptStageBoundary) || req.Cache.PrefixDigest == "" || req.Cache.Mode != "stable-prefix" {
		t.Fatalf("cache directive = %+v", req.Cache)
	}
	if req.Metadata["prompt_prefix_sha256"] != req.Cache.PrefixDigest || req.Metadata["prompt_cache_transport"] != "agentwrap-metadata-only" {
		t.Fatalf("cache metadata = %+v", req.Metadata)
	}
	if req.Metadata["prompt_optional_inputs"] != "" {
		t.Fatalf("optional input metadata = %+v", req.Metadata)
	}
}

func TestSprintRuntimeMetricsPersistCacheUsage(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-metrics")
	runtime := metricsRuntime{result: pruntime.Result{
		RunID: "run-1", SessionID: "session-1", Status: "success",
		Usage: pruntime.Usage{InputTokensKnown: true, InputTokens: 100, CacheReadTokensKnown: true, CacheReadTokens: 80, CacheWriteTokensKnown: true, CacheWriteTokens: 10},
	}}
	service := NewService(root).WithRuntime(runtime)
	prompt := "stable" + sharedPromptStageBoundary + "stage"
	req := service.runtimeRequest(prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StagePlan)})
	if _, err := service.startSprintRuntime(context.Background(), sp, StagePlan, req); err != nil {
		t.Fatal(err)
	}
	metrics, err := LoadRuntimeMetrics(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics.Runs) != 1 || !metrics.Runs[0].CacheReadTokens.Known || metrics.Runs[0].CacheReadTokens.Value != 80 || metrics.Runs[0].SharedPrefixDigest == "" || metrics.Runs[0].CacheKey == "" {
		t.Fatalf("persisted metrics = %+v", metrics)
	}
}

func TestSprintRuntimeMetricsPersistQACallIdentityAndToolAccounting(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-qa-metrics")
	runtime := metricsRuntime{result: pruntime.Result{
		RunID: "run-qa", SessionID: "session-qa", TurnID: "turn-qa", Status: "success",
		Events:      []pruntime.Event{{Kind: "tool", Type: "tool.call", Payload: map[string]any{"tool_name": "read"}}},
		EventStats:  pruntime.EventStats{Total: 1, Retained: 1},
		Usage:       pruntime.Usage{InputTokensKnown: true, InputTokens: 120, OutputTokensKnown: true, OutputTokens: 30, CacheReadTokensKnown: true, CacheReadTokens: 90},
		Permissions: pruntime.PermissionSummary{Mode: "restricted", Default: "deny", AuditCount: 1},
	}}
	service := NewService(root).WithRuntime(runtime)
	prompt := "stable" + sharedPromptStageBoundary + "qa"
	req := service.runtimeRequest(prompt, map[string]string{
		"project": sp.Project, "sprint": sp.Slug, "stage": string(VerificationPhaseQA), "operation": "qa-arbitrate", "role": "arbiter",
		"qa_attempt": "qa-v1-attempt-123456789012345678901234", "operational_attempt": "attempt-7", "map": "qa-v1-map-123456789012345678901234",
		"arbiter_group": "arbiter-group-1", "task": "arbiter-group-1", "variant": "high",
	})
	if _, err := service.startSprintRuntime(context.Background(), sp, PlanningStage(VerificationPhaseQA), req); err != nil {
		t.Fatal(err)
	}
	metrics, err := LoadRuntimeMetrics(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics.Runs) != 1 {
		t.Fatalf("runtime calls = %d", len(metrics.Runs))
	}
	call := metrics.Runs[0]
	if call.Sequence != 1 || call.StageSequence != 1 || call.Role != "arbiter" || call.Operation != "qa-arbitrate" || call.ArbiterGroupID != "arbiter-group-1" || call.TurnID != "turn-qa" || call.Variant != "high" {
		t.Fatalf("call identity = %+v", call)
	}
	if call.ToolCalls != 1 || call.ToolCallsByKind["read"] != 1 || !call.ToolCallCountExact || call.RuntimeEvents != 1 || call.RetainedEvents != 1 || call.PermissionMode != "restricted" || call.PermissionDefault != "deny" {
		t.Fatalf("call accounting = %+v", call)
	}
	if !call.InputTokens.Known || call.InputTokens.Value != 120 || !call.OutputTokens.Known || call.OutputTokens.Value != 30 || !call.CacheReadTokens.Known || call.CacheReadTokens.Value != 90 {
		t.Fatalf("call tokens = %+v", call)
	}
}

func TestSprintRuntimeMetricsUpgradeSchemaOneWithoutDroppingCalls(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-old-metrics")
	legacy := `{"schema_version":1,"project":"proj","sprint":"01-old-metrics","runs":[{"stage":"plan","run_id":"old","input_tokens":{"known":true,"value":41}}]}`
	writeFileContent(t, sp.Path, legacy, runtimeMetricsFileName)
	service := NewService(root).WithRuntime(metricsRuntime{result: pruntime.Result{RunID: "new", Status: "success"}})
	req := service.runtimeRequest("prompt", map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StagePlan)})
	if _, err := service.startSprintRuntime(context.Background(), sp, StagePlan, req); err != nil {
		t.Fatal(err)
	}
	metrics, err := LoadRuntimeMetrics(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.SchemaVersion != runtimeMetricsSchemaVersion || len(metrics.Runs) != 2 || metrics.Runs[0].Sequence != 1 || metrics.Runs[0].InputTokens.Value != 41 || metrics.Runs[1].Sequence != 2 {
		t.Fatalf("upgraded metrics = %+v", metrics)
	}
}

func TestRuntimeMetricsCanInspectSprintBeforePlanningInputsExist(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-empty-metrics")
	metrics, err := NewService(root).RuntimeMetrics("proj", "01")
	if err != nil || metrics.Project != sp.Project || metrics.Sprint != sp.Slug || len(metrics.Runs) != 0 {
		t.Fatalf("metrics=%+v err=%v", metrics, err)
	}
}

func TestRuntimeMetricsConcurrentWritersRemainValid(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-concurrent-metrics")
	service := NewService(root).WithRuntime(metricsRuntime{result: pruntime.Result{Status: "success"}})
	var wait sync.WaitGroup
	for i := 0; i < 16; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			req := service.runtimeRequest("stable"+sharedPromptStageBoundary+"stage", map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageReview)})
			_, _ = service.startSprintRuntime(context.Background(), sp, StageReview, req)
		}()
	}
	wait.Wait()
	metrics, err := LoadRuntimeMetrics(root, sp)
	if err != nil || len(metrics.Runs) != 16 {
		t.Fatalf("metrics runs=%d err=%v", len(metrics.Runs), err)
	}
}

type metricsRuntime struct{ result pruntime.Result }

func (r metricsRuntime) StartRun(context.Context, pruntime.Request) (pruntime.Result, error) {
	return r.result, nil
}

func TestAreaReasoningRunsOneMinimalRequestPerMissingTemplate(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	projectIndex := strings.Replace(testProjectIndex(), "| Architecture | .ultra/system/reasoning/architecture_reasoning_template.md | Boundaries |", "| Architecture | .ultra/system/reasoning/architecture_reasoning_template.md | Boundaries |\n| Security | .ultra/system/reasoning/security_reasoning_template.md | Threats |", 1)
	writeFileContent(t, root, projectIndex, "projects", "proj", "project-index.md")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeFileContent(t, root, "# Security Template\n", "system", "reasoning", "security_reasoning_template.md")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\n\nReason across two areas.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	sprintIndex := strings.Replace(validSprintIndex(), "| Architecture | projects/proj/sprints/01-alpha/reasoning/architecture.md | Boundaries |", "| Architecture | projects/proj/sprints/01-alpha/reasoning/architecture.md | Boundaries |\n| Security | projects/proj/sprints/01-alpha/reasoning/security.md | Threats |", 1)
	writeFileContent(t, sp.Path, sprintIndex, "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")

	runtime := &areaBatchRuntime{root: root}
	result, err := NewService(root).WithRuntime(runtime).FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageAreaReasoning})
	if err != nil || result.Message != "area-reasoning complete" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(runtime.requests) != 2 {
		t.Fatalf("runtime calls=%d want 2", len(runtime.requests))
	}
	for _, request := range runtime.requests {
		area := request.Metadata["area"]
		if area == "" || request.Metadata["output_path"] == "" {
			t.Fatalf("request missing area identity: %+v", request.Metadata)
		}
		other := "Security"
		if area == "Security" {
			other = "Architecture"
		}
		stageSuffix := request.Prompt[strings.Index(request.Prompt, sharedPromptStageBoundary)+len(sharedPromptStageBoundary):]
		if strings.Contains(stageSuffix, "# "+other+" Template") {
			t.Fatalf("%s request injected unrelated %s template", area, other)
		}
	}
}

func TestReviewerInputPacketExcludesSiblingCoverageSources(t *testing.T) {
	root, _ := reviewFixture(t)
	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "openai/gpt-test"}})
	manifest, findings, err := service.PrepareReview("proj", "01", ReviewRequest{})
	if err != nil || len(findings) != 0 || len(manifest.Coverage) < 2 {
		t.Fatalf("prepare review err=%v findings=%+v coverage=%d", err, findings, len(manifest.Coverage))
	}
	for _, coverage := range manifest.Coverage {
		packet := reviewerInputPacket(manifest, coverage)
		seenCoverage := false
		for _, input := range packet {
			if input.Path == coverage.Path {
				seenCoverage = true
			}
			if (input.Kind == "contract" || input.Kind == "handbook") && input.Path != coverage.Path {
				t.Fatalf("coverage %s received sibling source %s", coverage.ID, input.Path)
			}
		}
		if !seenCoverage {
			t.Fatalf("coverage %s did not receive its own source", coverage.ID)
		}
	}
}

type areaBatchRuntime struct {
	root           string
	requests       []pruntime.Request
	failAfterWrite bool
}

type batchExecutionRuntime struct {
	requests []pruntime.Request
}

type noSessionExecutionRuntime struct {
	requests []pruntime.Request
}

func (r *noSessionExecutionRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.requests = append(r.requests, req)
	return pruntime.Result{RunID: "execute-" + req.Metadata["task"], Status: "success", Artifacts: []pruntime.Artifact{{ID: "evidence", Kind: "test", Description: "task evidence"}}}, nil
}

type failingBatchExecutionRuntime struct {
	requests []pruntime.Request
}

func (r *failingBatchExecutionRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.requests = append(r.requests, req)
	return pruntime.Result{RunID: "execute-failed", Status: "failed"}, errors.New("task failed")
}

func (r *batchExecutionRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.requests = append(r.requests, req)
	if req.OnEvent != nil {
		req.OnEvent(pruntime.Event{SessionID: "execute-shared-session"})
	}
	return pruntime.Result{
		RunID: "execute-" + req.Metadata["task"], SessionID: "execute-shared-session", Status: "success",
		Artifacts: []pruntime.Artifact{{ID: "evidence-" + req.Metadata["task"], Kind: "test", Description: "task evidence"}},
	}, nil
}

func (r *areaBatchRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	r.requests = append(r.requests, req)
	area := req.Metadata["area"]
	content := validAreaReasoning()
	if area == "Security" {
		content = strings.ReplaceAll(content, "Architecture", "Security")
		content = strings.ReplaceAll(content, "architecture_reasoning_template.md", "security_reasoning_template.md")
	}
	path := filepath.Join(r.root, filepath.FromSlash(req.Metadata["output_path"]))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return pruntime.Result{}, err
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return pruntime.Result{}, err
	}
	if r.failAfterWrite {
		return pruntime.Result{RunID: "area-" + strings.ToLower(area), SessionID: "session-" + strings.ToLower(area), Status: "failed"}, errors.New("runtime exited before final result")
	}
	return pruntime.Result{RunID: "area-" + strings.ToLower(area), SessionID: "session-" + strings.ToLower(area), Status: "success"}, nil
}

func TestAreaReasoningAcceptsValidArtifactWhenRuntimeExitsBeforeFinalResult(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFileContent(t, root, testProjectIndex(), "projects", "proj", "project-index.md")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\n\nReason about architecture.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	writeFileContent(t, sp.Path, validReasoningTechnicalHandbook(), "technical-handbook.md")

	runtime := &areaBatchRuntime{root: root, failAfterWrite: true}
	result, err := NewService(root).WithRuntime(runtime).FlowReasoning(context.Background(), "proj", "01", FlowRequest{To: StageAreaReasoning})
	if err != nil || result.Message != "area-reasoning complete" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if len(runtime.requests) != 1 {
		t.Fatalf("runtime calls=%d want 1", len(runtime.requests))
	}
	validation, err := NewService(root).ValidateAreaReasoning("proj", "01")
	if err != nil || !validation.Valid() {
		t.Fatalf("recovered artifact validation=%+v err=%v", validation, err)
	}
}
