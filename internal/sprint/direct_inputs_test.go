package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testDigest(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func TestPlanningPromptsDirectlyInjectAcceptedProjectReasoning(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "03-current")
	writeFixtureProjectIndex(t, root, "proj")
	base := filepath.Join(root, "projects", "proj", "project-reasoning")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	index := "## Reasoning Areas\n\n| Area | Template | Output | Required | Depends On | Why |\n| --- | --- | --- | --- | --- | --- |\n\n## Evidence Assignments\n\n| Area | Evidence | Relevant Questions | Why Assigned |\n| --- | --- | --- | --- |\n\n## Source Document Assignments\n\n| Area | Source | Authority | Why Assigned |\n| --- | --- | --- | --- |\n\n## Excluded Evidence\n\n| Source | Reason Excluded | Revisit Trigger |\n| --- | --- | --- |\n"
	synthesis := "# Accepted project synthesis\n\nUnique governing project constraint.\n"
	review := "# Review\n\nVerdict: pass\n"
	writeFileContent(t, base, index, "index.md")
	writeFileContent(t, base, synthesis, "reasoning.md")
	writeFileContent(t, base, review, "review.md")
	state := fmt.Sprintf(`{"schema_version":1,"artifacts":{"reasoning":{"stage":"reasoning","output":"projects/proj/project-reasoning/reasoning.md","inputs":[],"output_sha256":"%s","completed_at":"2026-01-01T00:00:00Z"},"review":{"stage":"review","output":"projects/proj/project-reasoning/review.md","inputs":[],"output_sha256":"%s","completed_at":"2026-01-01T00:00:00Z"}},"verdict":"pass"}`, testDigest(synthesis), testDigest(review))
	writeFileContent(t, base, state, "flow-state.json")
	preview, err := NewService(root).PromptRequirements("proj", sp.Slug)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"ID: project-reasoning-synthesis", "Kind: accepted-project-reasoning", "Unique governing project constraint."} {
		if !strings.Contains(preview.Prompt, want) {
			t.Fatalf("requirements prompt missing %q:\n%s", want, preview.Prompt)
		}
	}
	inputs := directAcceptedProjectReasoningInputs(root, sp, true)
	packet := appendDirectInputPacket("stage\n", inputs)
	for _, want := range []string{"ID: project-reasoning-index", "ID: project-reasoning-synthesis", "Mode: full"} {
		if !strings.Contains(packet, want) {
			t.Fatalf("planning packet missing %q:\n%s", want, packet)
		}
	}
}

func TestDirectInputPacketPreservesCanonicalOrderAndExplanation(t *testing.T) {
	prompt := "stage instructions\n"
	inputs := []directPromptInput{
		directContentInput("first", "artifact", "first.md", "first content\n"),
		directContentInput("second", "project-doc", "docs/second.md", "second content\n"),
	}
	got := appendDirectInputPacket(prompt, inputs)
	if first, second := strings.Index(got, "ID: first"), strings.Index(got, "ID: second"); first < 0 || second <= first {
		t.Fatalf("direct input order was not preserved:\n%s", got)
	}
	if strings.Count(got, "Mode: full") != 2 {
		t.Fatalf("unexpected full packet size=%d:\n%s", len(got), got)
	}
	explanation := explainComposedPrompt(got)
	if explanation.TotalBytes != len(got) || len(explanation.Blocks) != 3 {
		t.Fatalf("explanation = %+v", explanation)
	}
	if explanation.Blocks[1].ID != "first" || explanation.Blocks[1].Kind != "artifact" || explanation.Blocks[1].Mode != "full" || explanation.Blocks[2].ID != "second" || explanation.Blocks[2].Mode != "full" {
		t.Fatalf("direct input explanations = %+v", explanation.Blocks)
	}
}

func TestDirectInputPacketIncludesOversizedInputsInFull(t *testing.T) {
	prompt := strings.Repeat("p", 128)
	inputs := []directPromptInput{
		directContentInput("one", "artifact", "one.md", strings.Repeat("α-one\n", 6000)),
		directContentInput("two", "artifact", "two.md", strings.Repeat("β-two\n", 6000)),
		directContentInput("three", "artifact", "three.md", strings.Repeat("γ-three\n", 6000)),
	}
	got := appendDirectInputPacket(prompt, inputs)
	for _, id := range []string{"one", "two", "three"} {
		if !strings.Contains(got, "ID: "+id) {
			t.Fatalf("oversized input %q was starved:\n%s", id, got)
		}
	}
	if strings.Count(got, "Mode: full") != len(inputs) || strings.Contains(got, "omitted by UltraPlan") {
		t.Fatalf("packet did not preserve every input in full:\n%s", got)
	}
}

func TestDirectInputPacketRedactsWorkspaceFromUnavailableInput(t *testing.T) {
	root := t.TempDir()
	input := directWorkspaceInput(root, "missing", "artifact", "projects/proj/missing.md")
	got := appendDirectInputPacket("stage\n", []directPromptInput{input})
	if strings.Contains(got, root) || strings.Contains(got, filepath.ToSlash(root)) {
		t.Fatalf("packet leaked workspace root: %s", got)
	}
	if !strings.Contains(got, "missing") || !strings.Contains(got, "not found") {
		t.Fatalf("packet omitted safe unavailable-input diagnostic: %s", got)
	}
}

func TestRequirementsDirectlyInjectsPriorReviewsInStableOrder(t *testing.T) {
	root := workspaceFixture(t)
	sp := sprintFixture(t, root, "proj", "03-current")
	writeFixtureProjectIndex(t, root, "proj")
	writeFileContent(t, root, "# First review\n", "projects", "proj", "sprints", "01-first", "review.md")
	writeFileContent(t, root, "# Second review\n", "projects", "proj", "sprints", "02-second", "review.md")
	writeFileContent(t, root, "# Future review must not be reused\n", "projects", "proj", "sprints", "04-future", "review.md")
	writeFileContent(t, sp.Path, "# Current review must not be reused\n", "review.md")
	preview, err := NewService(root).PromptRequirements("proj", "03")
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Index(preview.Prompt, "ID: prior-review-01-first")
	second := strings.Index(preview.Prompt, "ID: prior-review-02-second")
	if first < 0 || second <= first || strings.Contains(preview.Prompt, "Current review must not be reused") || strings.Contains(preview.Prompt, "Future review must not be reused") {
		t.Fatalf("prior review packet order/content is wrong:\n%s", preview.Prompt)
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "requirements.md")); !os.IsNotExist(err) {
		t.Fatalf("read-only prompt preview unexpectedly wrote requirements.md: %v", err)
	}
}

func TestSmokeAuthorDirectlyInjectsCompletePriorStageArtifacts(t *testing.T) {
	root, sp, service := executePersistenceFixture(t, nil)
	writeFileContent(t, sp.Path, "# Execute evidence\n", "execute.md")
	writeFileContent(t, sp.Path, "# Review outcome\n", "review.md")
	writeFileContent(t, sp.Path, "{\"status\":\"complete\"}\n", ".run-state.json")
	prepared := smokePrepared{
		Sprint: sp,
		Manifest: smokeManifest{
			Authoring: smokeAuthoring{Paths: []string{"tests"}},
		},
		HarnessRoot:  filepath.Join(root, "harness"),
		ManifestPath: filepath.Join(root, "harness", "manifest.json"),
		Target:       filepath.Join(root, "target"),
	}
	prompt := service.renderSmokeAuthorPrompt(prepared)
	for _, id := range []string{"sprint-index", "technical-handbook", "area-reasoning-architecture-md", "reasoning", "plan", "execute", "review", "execution-handoff"} {
		if !strings.Contains(prompt, "ID: "+id) {
			t.Fatalf("smoke prompt missing direct %s input", id)
		}
	}
	if strings.Contains(prompt, "omitted by UltraPlan") {
		t.Fatalf("smoke stage prompt truncated governed input: %s", prompt)
	}
}

func TestReviewPreviewUsesCompactReviewInputsAndFrozenTargetPaths(t *testing.T) {
	root, _ := reviewFixture(t)
	service := NewService(root).WithStageRuntime(map[PlanningStage]StageRuntime{StageReview: {Model: "test/model"}})
	if _, findings, prepareErr := service.PrepareReview("proj", "01", ReviewRequest{}); prepareErr != nil || len(findings) > 0 {
		t.Fatalf("prepare review err=%v findings=%+v", prepareErr, findings)
	}
	preview, err := service.PromptReview("proj", "01", ReviewRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Explanation == nil || preview.Explanation.SchemaVersion != promptExplanationSchemaVersion {
		t.Fatalf("review explanation = %+v", preview.Explanation)
	}
	direct := 0
	for _, block := range preview.Explanation.Blocks {
		if block.Mode != "" {
			direct++
			if block.Kind == "target" || block.ID == "run-state" || block.ID == "execute-run-state" {
				t.Fatalf("review suffix embedded raw target or run state: %+v", block)
			}
		}
	}
	if direct == 0 || !strings.Contains(preview.Prompt, directInputOpen) || !strings.Contains(preview.Prompt, "Frozen input index:") {
		t.Fatalf("review preview omitted compact inputs or frozen index")
	}
}
