package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/platform/config"
	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func TestSprintHelpIsRegistered(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"--help"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "sprint")
	stdout, stderr, status = runForTest([]string{"sprint", "--help"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("sprint help status = %d stderr = %q", status, stderr)
	}
	assertContains(t, stdout, "execute")
	for _, args := range [][]string{
		{"sprint", "--help"},
		{"sprint", "proj", "01", "status", "--help"},
		{"sprint", "proj", "01", "execute", "--help"},
		{"sprint", "proj", "01", "review", "--help"},
	} {
		stdout, stderr, status = runForTest(args)
		if status != ExitOK || stderr != "" {
			t.Fatalf("%v status = %d stderr = %q", args, status, stderr)
		}
		assertContains(t, stdout, "ultraplan sprint")
		if len(args) > 3 && args[2] == "01" && args[3] == "review" {
			assertContains(t, stdout, "--focus <coverage-id>")
		}
	}
}

func TestParseSprintExecuteDeferralRequiresTaskAndReason(t *testing.T) {
	req, err := parseSprintExecuteArgs([]string{"--task", "task-123", "--defer", "--reason", "accepted follow-up"})
	if err != nil || req.TaskID != "task-123" || req.DeferReason != "accepted follow-up" {
		t.Fatalf("req=%+v err=%v", req, err)
	}
	for _, args := range [][]string{
		{"--defer", "--reason", "why"},
		{"--task", "task-123", "--defer"},
		{"--task", "task-123", "--defer", "--reason", "why", "--resume"},
	} {
		if _, err := parseSprintExecuteArgs(args); err == nil {
			t.Fatalf("expected invalid deferral arguments: %v", args)
		}
	}
}

func TestSprintStatusRefreshesStateAndRendersDeterministically(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, commandValidRequirements(), "requirements.md")
	writeCommandCompletedCodeContext(t, dir, "proj", "01-alpha")
	writeFixtureFileContent(t, base, "# Sprint Index\n\nNo reasoning templates selected.\n", "sprint-index.md")
	writeFixtureFileContent(t, base, "# Handbook\n", "technical-handbook.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "status"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Project: proj\n")
	assertContains(t, stdout, "Sprint: 01-alpha\n")
	assertContains(t, stdout, "Flow state: projects/proj/sprints/01-alpha/flow-state.json\n")
	assertInOrder(t, stdout, "  requirements: complete", "  code-context: complete")
	assertInOrder(t, stdout, "  code-context: complete", "  sprint-index: complete")
	assertInOrder(t, stdout, "  sprint-index: complete", "  technical-handbook: complete")
	assertInOrder(t, stdout, "  technical-handbook: complete", "  area-reasoning: skipped")
	assertInOrder(t, stdout, "  area-reasoning: skipped", "  reasoning: ready")
	assertInOrder(t, stdout, "  reasoning: ready", "  plan: missing")
	if strings.Contains(stdout+stderr, "\x1b[") {
		t.Fatalf("unexpected ANSI escape sequence")
	}
	if _, err := os.Stat(filepath.Join(base, "flow-state.json")); err != nil {
		t.Fatalf("flow state not written: %v", err)
	}
	jsonOutput, jsonStderr, jsonStatus := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "status", "--json"})
	if jsonStatus != ExitOK || jsonStderr != "" {
		t.Fatalf("json status=%d stdout=%q stderr=%q", jsonStatus, jsonOutput, jsonStderr)
	}
	var envelope struct {
		SchemaVersion int `json:"schema_version"`
		Result        struct {
			Stages []struct {
				Stage string `json:"stage"`
			} `json:"stages"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &envelope); err != nil || envelope.SchemaVersion != 1 || len(envelope.Result.Stages) < 3 || envelope.Result.Stages[1].Stage != "code-context" {
		t.Fatalf("stable status projection=%+v err=%v body=%s", envelope, err, jsonOutput)
	}
}

func TestSprintFailureJSONIsOneStructuredDocument(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	for _, args := range [][]string{
		{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "plan", "--dry-run", "--json"},
		{"--workspace", dir, "sprint", "proj", "01", "verify", "--to", "review", "--json"},
	} {
		stdout, _, status := runForTest(args)
		if status == ExitOK {
			t.Fatalf("expected failed command: %v stdout=%s", args, stdout)
		}
		decoder := json.NewDecoder(strings.NewReader(stdout))
		var payload map[string]any
		if err := decoder.Decode(&payload); err != nil {
			t.Fatalf("invalid JSON for %v: %v\n%s", args, err, stdout)
		}
		var trailing any
		if err := decoder.Decode(&trailing); err == nil {
			t.Fatalf("trailing JSON/output for %v: %s", args, stdout)
		}
		if payload["status"] == "complete" || payload["status"] == "ready" {
			t.Fatalf("failure reported success: %#v", payload)
		}
		if args[5] == "verify" && payload["error"] == nil {
			t.Fatalf("verify failure omitted structured error: %#v", payload)
		}
	}
}

func TestSprintStatusErrorsAndInvalidFlowStateExitFive(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "api", "01-alpha")
	writeCommandSprintProject(t, dir, "api", "02-alpha")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "api", "0", "status"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("ambiguous status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `ambiguous sprint reference "0"`)

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "missing", "01", "status"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("missing project status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, `project reference "missing" not found`)

	base := filepath.Join(dir, "projects", "api", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, `{"schemaVersion":1}`, "flow-state.json")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "api", "01-alpha", "status"})
	if status != ExitValidation || stdout != "" {
		t.Fatalf("invalid state status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "flow state malformed")
	content, err := os.ReadFile(filepath.Join(base, "flow-state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != `{"schemaVersion":1}` {
		t.Fatalf("invalid state was overwritten: %s", content)
	}
}

func TestSprintMalformedArgumentsUseUsageExit(t *testing.T) {
	stdout, stderr, status := runForTest([]string{"sprint", "proj", "status"})
	if status != ExitUsage || stdout != "" {
		t.Fatalf("status = %d stdout = %q stderr = %q", status, stdout, stderr)
	}
	assertContains(t, stderr, "expected '<project> <sprint> status'")
}

func TestSprintValidatePromptAndDryRunCommands(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, commandValidRequirements(), "requirements.md")
	writeCommandCompletedCodeContext(t, dir, "proj", "01-alpha")
	writeFixtureFileContent(t, base, commandValidSprintIndex(), "sprint-index.md")
	writeFixtureFileContent(t, base, commandValidTechnicalHandbook(), "technical-handbook.md")
	writeFixtureFileContent(t, filepath.Join(dir, "projects", "proj"), commandProjectIndex(), "project-index.md")
	writeFixtureFileContent(t, dir, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFixtureFileContent(t, dir, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "code-context"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("code-context validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "code-context.md")
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "code-context"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("code-context prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Sprint Code Context")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-code-context.md`")
	assertContains(t, stdout, "Source: builtin:templates/code-context.md")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "sprint-index"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "technical-handbook"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("handbook validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "technical-handbook.md")
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "sprint-index"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Sprint Index")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-sprint-index.md`")
	assertContains(t, stdout, "Injected Sprint Index Template:")
	assertContains(t, stdout, "Source: builtin:templates/sprint-index.md")
	assertContains(t, stdout, "Do not mutate")
	if strings.Contains(stdout+stderr, "\x1b[") || strings.Contains(stdout, dir) {
		t.Fatalf("unsafe prompt output stdout=%q stderr=%q", stdout, stderr)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "technical-handbook"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("handbook prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Technical Handbook")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-technical-handbook.md`")
	assertContains(t, stdout, "Selected evidence:")
	if strings.Contains(stdout+stderr, "\x1b[") || strings.Contains(stdout, dir) {
		t.Fatalf("unsafe handbook prompt output stdout=%q stderr=%q", stdout, stderr)
	}

	stateBefore, stateBeforeErr := os.ReadFile(filepath.Join(base, "flow-state.json"))
	if stateBeforeErr != nil {
		t.Fatal(stateBeforeErr)
	}
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "sprint-index", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("flow dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Dry run: true")
	stateAfter, stateErr := os.ReadFile(filepath.Join(base, "flow-state.json"))
	if stateErr != nil || string(stateAfter) != string(stateBefore) {
		t.Fatalf("dry run changed state: %v", stateErr)
	}
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "code-context", "--dry-run"})
	if status != ExitOK || stderr != "" || !strings.Contains(stdout, "Dry run: true") {
		t.Fatalf("code-context dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	stateAfter, stateErr = os.ReadFile(filepath.Join(base, "flow-state.json"))
	if stateErr != nil || string(stateAfter) != string(stateBefore) {
		t.Fatalf("code-context dry-run changed state: %v", stateErr)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "technical-handbook", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("handbook flow dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Flow target: technical-handbook")
	assertContains(t, stdout, "Dry run: true")

	writeFixtureFileContent(t, base, commandValidAreaReasoning(), "reasoning", "architecture.md")
	writeFixtureFileContent(t, base, commandValidReasoning(), "reasoning.md")
	writeFixtureFileContent(t, base, commandValidPlan(), "plan.md")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "area-reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("area validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("reasoning validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "reasoning.md")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "plan"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("plan validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "plan.md")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "execute"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("execute validate status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: ok")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "area-reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("area prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Area Reasoning")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-area-reasoning.md`")
	assertContains(t, stdout, "## Area Decisions")
	assertContains(t, stdout, "## Trade-Offs")
	assertContains(t, stdout, "## Evidence")
	assertContains(t, stdout, "## Risks")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "reasoning"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("reasoning prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Create Sprint Reasoning")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/create-sprint-reasoning.md`")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "plan"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("plan prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Sprint Planning - Evidence-Grounded Implementation Plan")
	assertContains(t, stdout, "Prompt source: `builtin:prompts/plan-sprint.md`")
	assertContains(t, stdout, "Do not execute implementation tasks")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "prompt", "execute"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("execute prompt status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "# Execute Sprint Task")
	assertContains(t, stdout, "Approved target")
	assertContains(t, stdout, "Do not run or request Git mutation")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "reasoning", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("reasoning dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Flow target: reasoning")
	assertContains(t, stdout, "Dry run: true")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "plan", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("plan dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Flow target: plan")
	assertContains(t, stdout, "Dry run: true")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "execute", "--dry-run"})
	if status != ExitOK || stderr != "" {
		t.Fatalf("execute dry-run status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Flow target: execute")
	assertContains(t, stdout, "# Execute Sprint Task")
}

func TestSprintFlowNonDryRunUsesConfiguredRuntime(t *testing.T) {
	dir := initializedWorkspace(t)
	writeFixtureFileContent(t, dir, `version: 1
runtime:
  default: opencode
models:
  default: minimax-coding-plan/MiniMax-M3
  primary: minimax-coding-plan/MiniMax-M3
  backup: minimax-coding-plan/MiniMax-M3
execution:
  default_variant: high
  default_parallel: 1
  default_timeout: 12m
  default_retries: 1
planning:
  requirements_model: openai/gpt-5.5
  requirements_variant: high
  code_context_model: openai/gpt-5.5
  code_context_variant: high
  sprint_index_model: openai/gpt-5.5
  sprint_index_variant: high
  reasoning_model: openai/gpt-5.5
  reasoning_variant: high
  plan_model: openai/gpt-5.5
  plan_variant: high
logging:
  format: text
  level: info
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
`, "ultraplan.yml")
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, "# Requirements\n\nSelect stage.\n", "requirements.md")
	writeFixtureFileContent(t, base, commandValidSprintIndex(), "sprint-index.md")
	writeFixtureFileContent(t, filepath.Join(dir, "projects", "proj"), commandProjectIndex(), "project-index.md")
	writeFixtureFileContent(t, dir, "# Evidence\n", "studies", "go-cli-study", "reports", "final", "01-project-structure.md")
	writeFixtureFileContent(t, dir, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")

	fake := &sprintCommandRuntime{}
	restore := stubSprintRuntimeFactory(fake)
	defer restore()

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "sprint-index"})
	if status != ExitOK {
		t.Fatalf("flow status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stderr, "[sprint] requirements")
	assertContains(t, stderr, "running  starting runtime-backed stage")
	assertContains(t, stderr, "[runtime] requirements")
	assertContains(t, stderr, "lifecycle.transition state=running")
	assertContains(t, stdout, "Result: sprint-index already complete")
	if fake.calls != 2 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	runJSON, runStderr, runStatus := runForTest([]string{"--workspace", dir, "run", "list", "--json"})
	if runStatus != ExitOK || runStderr != "" {
		t.Fatalf("run list status=%d stderr=%q output=%q", runStatus, runStderr, runJSON)
	}
	assertContains(t, runJSON, `"kind": "operation"`)
	assertContains(t, runJSON, `"operation": "sprint-flow"`)
	if fake.request.Provider == "" || fake.request.Model == "" {
		t.Fatalf("runtime request did not include config: %+v", fake.request)
	}
	if fake.request.Provider != "openai" || fake.request.Model != "gpt-5.5" {
		t.Fatalf("planning model override was not used: %+v", fake.request)
	}
	if fake.request.Metadata["reasoning_effort"] != "high" {
		t.Fatalf("planning variant metadata was not used: %+v", fake.request.Metadata)
	}
	if fake.request.Metadata["stage"] != "code-context" {
		t.Fatalf("runtime metadata = %+v", fake.request.Metadata)
	}
	assertContains(t, fake.request.Prompt, "# Create Sprint Code Context")
	assertContains(t, fake.request.Prompt, "Prompt source: `builtin:prompts/create-code-context.md`")

	writeFixtureFileContent(t, base, commandValidTechnicalHandbook(), "technical-handbook.md")
	writeFixtureFileContent(t, base, commandValidAreaReasoning(), "reasoning", "architecture.md")
	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "reasoning"})
	if status != ExitOK {
		t.Fatalf("reasoning flow status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stderr, "[runtime] reasoning")
	assertContains(t, stdout, "Result: reasoning complete")
	if fake.calls != 3 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if fake.request.Metadata["stage"] != "reasoning" {
		t.Fatalf("runtime metadata = %+v", fake.request.Metadata)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "plan"})
	if status != ExitOK {
		t.Fatalf("plan flow status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stderr, "[runtime] plan")
	assertContains(t, stdout, "Result: plan complete")
	if fake.calls != 4 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if fake.request.Metadata["stage"] != "plan" {
		t.Fatalf("runtime metadata = %+v", fake.request.Metadata)
	}

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "execute"})
	if status != ExitOK {
		t.Fatalf("execute flow status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stderr, "[runtime] execute")
	assertContains(t, stdout, "Result: execute complete")
	if fake.calls != 5 {
		t.Fatalf("runtime calls = %d", fake.calls)
	}
	if fake.request.Metadata["stage"] != "execute" {
		t.Fatalf("runtime metadata = %+v", fake.request.Metadata)
	}
	privateDir := filepath.Join(dir, ".ultraplan")
	if err := os.Rename(privateDir, privateDir+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(privateDir, []byte("repository unavailable"), 0o600); err != nil {
		t.Fatal(err)
	}
	startedBeforeFailure := fake.calls
	_, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "plan"})
	if status != ExitRuntime || fake.calls != startedBeforeFailure {
		t.Fatalf("persistence failure status=%d calls=%d want=%d stderr=%q", status, fake.calls, startedBeforeFailure, stderr)
	}
	assertContains(t, stderr, "run-control")
}

func TestSprintValidateFailuresAndUnsupportedStages(t *testing.T) {
	dir := initializedWorkspace(t)
	writeCommandSprintProject(t, dir, "proj", "01-alpha")
	base := filepath.Join(dir, "projects", "proj", "sprints", "01-alpha")
	writeFixtureFileContent(t, base, "# Requirements\n\nSelect stage.\n", "requirements.md")
	writeFixtureFileContent(t, base, "# Sprint Index\n\nTODO\n", "sprint-index.md")
	writeFixtureFileContent(t, filepath.Join(dir, "projects", "proj"), commandProjectIndex(), "project-index.md")

	stdout, stderr, status := runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "validate", "sprint-index"})
	if status != ExitValidation {
		t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stdout, "Validation: failed")
	assertContains(t, stderr, "sprint-index validation failed")

	stdout, stderr, status = runForTest([]string{"--workspace", dir, "sprint", "proj", "01", "flow", "--to", "smoke"})
	if status != ExitUsage || stdout != "" {
		t.Fatalf("unsupported status=%d stdout=%q stderr=%q", status, stdout, stderr)
	}
	assertContains(t, stderr, "--yes is required for smoke execution")
}

func TestParseSprintReviewArgs(t *testing.T) {
	req, jsonOut, err := parseSprintReviewArgs([]string{"--dry-run", "--restart", "--model", "openai/gpt-5.6", "--parallel", "4", "--json"})
	if err != nil || !req.DryRun || !req.Restart || req.ModelOverride != "openai/gpt-5.6" || req.Concurrency != 4 || !jsonOut {
		t.Fatalf("req=%+v json=%t err=%v", req, jsonOut, err)
	}
	if _, _, err := parseSprintReviewArgs([]string{"--parallel", "0"}); err == nil {
		t.Fatal("expected invalid parallelism")
	}
	if _, _, err := parseSprintReviewArgs([]string{"--restart", "--focus", "contract-testing"}); err == nil {
		t.Fatal("expected restart/focus conflict")
	}
	for _, want := range []string{"validate review", "prompt review", "flow --to review", "review [--restart]"} {
		if !strings.Contains(sprintHelp(), want) {
			t.Fatalf("help missing %q", want)
		}
	}
}

func TestParseSprintFlowCodeContextOverrides(t *testing.T) {
	req, err := parseSprintFlowArgs([]string{"--to", "code-context", "--dry-run", "--model", "vendor/context", "--variant", "max"})
	if err != nil || req.To != sprint.StageCodeContext || !req.DryRun || req.ModelOverride != "vendor/context" || req.VariantOverride != "max" {
		t.Fatalf("req=%+v err=%v", req, err)
	}
	if _, err := parseSprintFlowArgs([]string{"--to", "code-context", "--model"}); err == nil {
		t.Fatal("expected missing model value error")
	}
}

func TestPlanningStageRuntimeCodeContextFallback(t *testing.T) {
	c := config.Defaults()
	c.Models.Primary = "primary/context"
	c.Execution.DefaultVariant = "high"
	runtime := planningStageRuntime(c)[sprint.StageCodeContext]
	if runtime.Model != "primary/context" || runtime.Variant != "high" {
		t.Fatalf("fallback runtime = %+v", runtime)
	}
	c.Planning.CodeContextModel = "stage/context"
	c.Planning.CodeContextVariant = "max"
	runtime = planningStageRuntime(c)[sprint.StageCodeContext]
	if runtime.Model != "stage/context" || runtime.Variant != "max" {
		t.Fatalf("stage runtime = %+v", runtime)
	}
}

func TestParseSprintSmokeArgsAndHelp(t *testing.T) {
	req, jsonOut, err := parseSprintSmokeArgs([]string{"--suite", "sprint-27", "--timeout", "2m", "--force-review", "--override-reason", "diagnose blocked review", "--dry-run", "--yes", "--json"})
	if err != nil || req.Suite != "sprint-27" || req.Timeout != 2*time.Minute || !req.ForceReview || req.OverrideRationale == "" || !req.DryRun || !req.NonInteractive || !jsonOut {
		t.Fatalf("req=%+v json=%t err=%v", req, jsonOut, err)
	}
	if _, _, err := parseSprintSmokeArgs([]string{"--suite", "a", "--test", "b"}); err == nil {
		t.Fatal("expected exclusive scope error")
	}
	if _, _, err := parseSprintSmokeArgs([]string{"--force-review", "--yes"}); err == nil {
		t.Fatal("expected override rationale error")
	}
	for _, want := range []string{"validate smoke", "smoke [--level", "--force-review", "--dry-run"} {
		if !strings.Contains(sprintHelp()+sprintSmokeHelp(), want) {
			t.Fatalf("help missing %q", want)
		}
	}
}

func writeCommandSprintProject(t *testing.T, root, projectName, sprintSlug string) {
	t.Helper()
	base := filepath.Join(root, "projects", projectName)
	mkdirAll(t, base, "docs")
	mkdirAll(t, base, "sprints", sprintSlug)
	writeFixtureFileContent(t, base, "# PRD\n", "docs", "PRD.md")
	writeFixtureFileContent(t, base, "# Roadmap\n", "roadmap.md")
	writeFixtureFileContent(t, base, "# Project Index\n", "project-index.md")
}

func commandProjectIndex() string {
	return `# Project Index

## Project Scope

- **Target Implementation Directory:** /home/antonioborgerees/coding/ultraplan/ultraplan-go

## Active Contract Pool

| Contract | Path | Applies To |
|---|---|---|
| Architecture | .ultra/system/contracts/core/architecture.md | All sprints |

## Available Evidence Reports

| Report | Path | Covers |
|---|---|---|
| 01-project-structure | .ultra/studies/go-cli-study/reports/final/01-project-structure.md | Project layout |

## Available Reasoning Templates

| Template | Path | Useful For |
|---|---|---|
| Architecture | .ultra/system/reasoning/architecture_reasoning_template.md | Boundaries |

## Review Protocols

| Protocol | Path | Required When |
|---|---|---|
| Sprint Review | .ultra/system/protocols/sprint-review-protocol.md | Every sprint |
`
}

func commandValidSprintIndex() string {
	return `# Sprint Index

## Selected Contracts

| Contract | Why Selected |
|---|---|
| Architecture | Boundaries |

## Selected Evidence Reports

| Report | Path | Covers |
|---|---|---|
| 01-project-structure | .ultra/studies/go-cli-study/reports/final/01-project-structure.md | Project layout |

## Selected Reasoning Templates

| Template | Output Path | Why Selected |
|---|---|---|
| Architecture | projects/proj/sprints/01-alpha/reasoning/architecture.md | Boundaries |

## Required Review Protocols

| Protocol | Path | Required Evidence |
|---|---|---|
| Sprint Review | .ultra/system/protocols/sprint-review-protocol.md | Evidence |

## Excluded Context

| Context | Reason Excluded | Revisit If |
|---|---|---|
| Sprint implementation execution | deferred | future |
| Smoke investigation execution | deferred | future |
| Automated review | deferred | future |
| Issue tracking | deferred | future |
| Git mutation | deferred | future |
`
}

func commandValidRequirements() string {
	return `# Sprint Requirements: 01-alpha

> Project: proj
> Sprint: 01-alpha

## Sprint Goal

Select sprint context for the next planning stage.

## Required Outputs

| Output | Path | Description |
|---|---|---|
| Sprint index | projects/proj/sprints/01-alpha/sprint-index.md | Selected context |

## Acceptance Criteria

- [ ] Requirements are specific.

## Non-Goals

- Smoke investigation.

## Constraints

- Use workspace-relative paths.

## Dependencies

| Prior Sprint / Output | Required For | Notes |
|---|---|---|
| Project index | Planning | Must validate |

## Review Expectations

| What | How Verified |
|---|---|
| Requirements | Validation |
`
}

type sprintCommandRuntime struct {
	calls   int
	request runtimepkg.Request
	stages  []string
}

func (f *sprintCommandRuntime) StartRun(_ context.Context, req runtimepkg.Request) (runtimepkg.Result, error) {
	f.calls++
	f.request = req
	f.stages = append(f.stages, req.Metadata["stage"])
	if req.OnEvent != nil {
		req.OnEvent(runtimepkg.Event{Type: "lifecycle.transition", Kind: "lifecycle", Payload: map[string]any{"state": "running"}})
	}
	if req.Metadata["stage"] == string(sprint.StageRequirements) {
		path := filepath.Join(req.WorkDir, "projects", req.Metadata["project"], "sprints", req.Metadata["sprint"], "requirements.md")
		if err := os.WriteFile(path, []byte(commandValidRequirements()), 0o644); err != nil {
			return runtimepkg.Result{}, err
		}
	}
	if req.Metadata["stage"] == string(sprint.StageCodeContext) {
		return runtimepkg.Result{RunID: "context-run", Status: "completed", TerminalOutput: commandValidCodeContext()}, nil
	}
	if req.Metadata["stage"] == string(sprint.StageReasoning) {
		path := filepath.Join(req.WorkDir, "projects", req.Metadata["project"], "sprints", req.Metadata["sprint"], "reasoning.md")
		if err := os.WriteFile(path, []byte(commandValidReasoning()), 0o644); err != nil {
			return runtimepkg.Result{}, err
		}
	}
	if req.Metadata["stage"] == string(sprint.StagePlan) {
		path := filepath.Join(req.WorkDir, "projects", req.Metadata["project"], "sprints", req.Metadata["sprint"], "plan.md")
		if err := os.WriteFile(path, []byte(commandValidPlan()), 0o644); err != nil {
			return runtimepkg.Result{}, err
		}
	}
	if req.Metadata["stage"] == string(sprint.StageExecute) {
		return runtimepkg.Result{RunID: "execute-run", Status: "completed", Artifacts: []runtimepkg.Artifact{{ID: "execute-evidence", Kind: "test", Description: "execute fake runtime evidence"}}}, nil
	}
	return runtimepkg.Result{RunID: "sprint-run", Status: "completed"}, nil
}

func stubSprintRuntimeFactory(rt *sprintCommandRuntime) func() {
	orig := testSprintRuntimeFactory
	testSprintRuntimeFactory = func(config.Config) (sprint.Runtime, error) {
		return rt, nil
	}
	return func() { testSprintRuntimeFactory = orig }
}

func commandValidTechnicalHandbook() string {
	return `# Sprint Technical Handbook

## Selected Studies And Reports

| Study / Report | Path | Relevant Finding |
| --- | --- | --- |
| 01-project-structure | .ultra/studies/go-cli-study/reports/final/01-project-structure.md | Thin entrypoints. |

## Relevant Patterns

- Module-owned behavior.

## Trade-Offs

| Trade-Off | Benefit | Cost |
| --- | --- | --- |
| Local validation | Clear ownership | Focused parser |

## Anti-Patterns And Warnings

- Do not read unselected evidence.

## Open Questions For Reasoning

- How strict should validation be?

## Evidence Pointers

- .ultra/studies/go-cli-study/reports/final/01-project-structure.md
`
}

func commandValidCodeContext() string {
	return `# Sprint Code Context

## Sprint Scope

Implement the selected sprint.

## Inspected Repository Areas

- internal/sprint

## Selected Source References

### Service

- **Path:** ` + "`internal/sprint/service.go`" + `
- **Lines:** ` + "`1-20`" + `
- **Rationale:** The service owns sprint behavior.

## Relationships

App calls sprint services.

## Constraints

Source remains read-only.

## Open Questions

None.
`
}

func writeCommandCompletedCodeContext(t *testing.T, root, projectName, sprintSlug string) {
	t.Helper()
	base := filepath.Join(root, "projects", projectName, "sprints", sprintSlug)
	writeFixtureFileContent(t, base, commandValidCodeContext(), "code-context.md")
	sp := sprint.Sprint{Project: projectName, Slug: sprintSlug, Path: base}
	now := time.Now().UTC()
	states := make([]sprint.StageState, 0, len(sprint.PlanningStages()))
	for _, stage := range sprint.PlanningStages() {
		status := sprint.StatusMissing
		if stage == sprint.StageRequirements || stage == sprint.StageCodeContext {
			status = sprint.StatusComplete
		}
		if stage == sprint.StageSprintIndex {
			status = sprint.StatusReady
		}
		states = append(states, sprint.StageState{Stage: stage, Status: status, Path: sprint.ArtifactRelPath(sp, stage)})
	}
	if err := sprint.SaveFlowState(root, sp, sprint.NewFlowState(sp, states, now)); err != nil {
		t.Fatal(err)
	}
}

func commandValidAreaReasoning() string {
	return `# Architecture Reasoning

## Area Decisions

- Architecture uses .ultra/system/reasoning/architecture_reasoning_template.md.

## Trade-Offs

- Local validation keeps ownership clear.

## Evidence

- .ultra/studies/go-cli-study/reports/final/01-project-structure.md

## Risks

- Structural validation is limited.
`
}

func commandValidReasoning() string {
	return `# Sprint Reasoning

## Sprint Purpose

Implement reasoning.

## Selected Context And Pre-Reasoning Artifacts

- requirements.md

## Area-Specific Reasoning Inputs

- Architecture: projects/proj/sprints/01-alpha/reasoning/architecture.md

## Decisions

- Keep behavior in internal/sprint.

## Expected Evidence

- go test ./...

## Assumptions And Risks

- Structural validation has limits.

## Implementation Constraints

- Do not generate or validate plan.md.
`
}

func commandValidPlan() string {
	return `# Sprint Plan

## Reasoning Source

- Source: projects/proj/sprints/01-alpha/reasoning.md

## Sprint Status

- Status: not started

## Decisions To Execute

| Decision | Source |
|---|---|
| Keep behavior in internal/sprint | reasoning.md |

## Requirements / Contracts To Satisfy

| Contract | Evidence |
|---|---|
| AC-01 | go test ./... |

## Tasks

- [ ] Task 1: Add plan behavior for Decision 1 / AC-01
  > Executes: Decision 1, AC-01
  - [ ] Verification expectation: go test ./...

## Evidence Checklist

- [ ] Command tests pass.

## Risks And Blockers

| Risk | Mitigation |
|---|---|
| Structural validation | Keep checks focused. |

## Execution Log

| Step | Evidence |
|---|---|
| pending | pending |

## Completion Criteria

- [ ] Tests pass.
`
}
