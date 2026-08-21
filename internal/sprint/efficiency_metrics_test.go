package sprint

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

// TestSprintEfficiencyMetrics emits reproducible structural measurements used
// by docs/reports/sprint-efficiency.md. Keep the metric names stable so results
// from the baseline commit and the improved implementation remain comparable.
func TestSprintEfficiencyMetrics(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "01-alpha")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# Architecture Template\n", "system", "reasoning", "architecture_reasoning_template.md")
	writeEvidenceFile(t, root)
	writeFileContent(t, sp.Path, "# Requirements\n\nShared efficiency fixture.\n", "requirements.md")
	writeCompletedCodeContext(t, root, sp)
	writeFileContent(t, sp.Path, validSprintIndex(), "sprint-index.md")
	handbook := strings.Replace(validReasoningTechnicalHandbook(), "## Open Questions For Reasoning", "## Examples Worth Investigating\n\n- Inspect `internal/sprint/handoff.go` for the bounded handoff pattern.\n\n## Open Questions For Reasoning", 1)
	writeFileContent(t, sp.Path, handbook, "technical-handbook.md")
	writeFileContent(t, sp.Path, validAreaReasoning(), "reasoning", "architecture.md")
	writeFileContent(t, sp.Path, validPlanFinalReasoning(), "reasoning.md")
	writeFileContent(t, sp.Path, validPlan(), "plan.md")

	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageExecute: {Model: "test/model"}})
	previews := []struct {
		name string
		call func() (PromptPreview, error)
	}{
		{"sprint-index", func() (PromptPreview, error) { return service.PromptSprintIndex("proj", "01") }},
		{"technical-handbook", func() (PromptPreview, error) { return service.PromptTechnicalHandbook("proj", "01") }},
		{"area-reasoning", func() (PromptPreview, error) { return service.PromptAreaReasoning("proj", "01") }},
		{"reasoning", func() (PromptPreview, error) { return service.PromptReasoning("proj", "01") }},
		{"plan", func() (PromptPreview, error) { return service.PromptPlan("proj", "01") }},
		{"execute", func() (PromptPreview, error) { return service.PromptExecute("proj", "01", ExecuteRequest{}) }},
	}
	for _, item := range previews {
		preview, err := item.call()
		if err != nil {
			t.Fatalf("%s preview: %v", item.name, err)
		}
		prefix := testSharedPrefix(t, preview.Prompt)
		t.Logf("metric prompt.%s.total_bytes=%d", item.name, len(preview.Prompt))
		t.Logf("metric prompt.%s.prefix_bytes=%d", item.name, len(prefix))
		t.Logf("metric prompt.%s.suffix_bytes=%d", item.name, len(preview.Prompt)-len(prefix))
	}

	usage := pruntime.Usage{InputTokensKnown: true, InputTokens: 1000, OutputTokensKnown: true, OutputTokens: 100, CacheReadTokensKnown: true, CacheReadTokens: 700, CacheWriteTokensKnown: true, CacheWriteTokens: 200}
	summary := runtimeSummary(pruntime.Result{Usage: usage}, ExecuteModelSelection{Model: "test/model", Source: "test"})
	t.Logf("metric telemetry.execute_usage_summary_present=%t", strings.TrimSpace(summary.UsageSummary) != "")

	compatible := stageSessionCompatible(stageSessionRecord{SessionID: "session", Provider: "opencode", Model: "model", WorkDir: root, PromptChecksum: "old"}, pruntime.Request{Provider: "opencode", Model: "model", WorkDir: root, PromptRef: pruntime.PromptReference{Checksum: "new"}})
	t.Logf("metric resume.changed_prompt_compatible=%t", compatible)
}

func BenchmarkSharedPromptComposition(b *testing.B) {
	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte(strings.Repeat("selected source evidence\n", 300)), 0o644); err != nil {
		b.Fatal(err)
	}
	contextArtifact := validSharedCodeContext(
		sharedReference("First", "source.go", "1-100", "", "first selection"),
		sharedReference("Overlap", "source.go", "51-150", "", "overlapping selection"),
		sharedReference("Duplicate", "source.go", "1-100", "", "duplicate selection"),
	)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := renderSharedPromptContext(context.Background(), Sprint{Project: "proj", Slug: "01"}, "# Requirements\n", contextArtifact, root); err != nil {
			b.Fatal(err)
		}
	}
}
