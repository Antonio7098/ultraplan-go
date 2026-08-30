package sprint

import (
	"strings"
	"testing"
)

func TestQAFoundationCarriesExactAcceptanceSourceDiffAndProvenance(t *testing.T) {
	manifest := ReviewManifest{
		Project: "alpha", Sprint: "01-test", ChangedPaths: []string{"internal/app/usecases.go", "internal/sprint/qa.go", "internal/web/handlers.go"},
		Coverage: []ReviewInput{{ID: "REQ-MAP", Path: "contracts/map.md"}, {ID: "REQ-WEB", Path: "contracts/web.md"}},
		Inputs:   []ReviewInput{{ID: "requirements", Path: "requirements.md"}, {ID: "execution-handoff", Path: "handoff.md"}, {ID: "implementation-diff", Path: reviewPatchPath}},
		Contents: map[string]string{
			"contracts/map.md":                     "REQ-MAP: every changed path has one semantic owner.",
			"contracts/web.md":                     "REQ-WEB: pass the requested page limit unchanged.",
			"requirements.md":                      "# Requirements\nExact acceptance text.",
			"handoff.md":                           "# Execution handoff\nImplementation complete.",
			reviewPatchPath:                        "diff --git a/internal/web/handlers.go b/internal/web/handlers.go\n--- a/internal/web/handlers.go\n+++ b/internal/web/handlers.go\n@@ -1 +1 @@\n-old\n+new\n",
			"target/internal/app/usecases.go":      "package app\nfunc Use() {}\n",
			"target/internal/sprint/qa.go":         "package sprint\nfunc QA() {}\n",
			"target/internal/web/handlers.go":      "package web\nfunc Handle(limit int) int { return limit }\n",
			"target/internal/web/handlers_test.go": "package web\nfunc TestHandle() {}\n",
		},
	}
	foundation, err := BuildQAFoundation(manifest, strings.Repeat("1", 64), strings.Repeat("2", 64), strings.Repeat("3", 64), nil, DefaultQABudgets(), "# Review\nNo known findings.")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, block := range foundation.Blocks {
		kinds[block.Kind]++
		if block.ID == "" || block.ContentSHA256 == "" || block.Provenance == "" {
			t.Fatalf("incomplete block: %+v", block)
		}
	}
	for _, kind := range []string{"acceptance", "source", "change", "prior", "evidence", "authority"} {
		if kinds[kind] == 0 {
			t.Fatalf("foundation omitted %s blocks: %+v", kind, kinds)
		}
	}
	if len(foundation.Omissions) != 0 || !validFingerprint(foundation.Fingerprint) {
		t.Fatalf("foundation identity or omissions = %+v", foundation)
	}
}

func TestPackCompleteInvestigatorUsesStableFoundationAndNoRepositoryTools(t *testing.T) {
	input := qaMapInputFixture()
	manifest := ReviewManifest{Project: input.Project, Sprint: input.Sprint, ChangedPaths: input.ChangedPaths, Coverage: []ReviewInput{{ID: "REQ-MAP", Path: "map.md"}, {ID: "REQ-WEB", Path: "web.md"}}, Contents: map[string]string{"map.md": "REQ-MAP exact text", "web.md": "REQ-WEB exact text"}}
	for _, path := range input.ChangedPaths {
		manifest.Contents["target/"+path] = "package sample\nfunc ExactBehavior() {}\n"
	}
	manifest.Contents["target/internal/web/handlers_test.go"] = "package sample\nfunc TestExactBehavior() {}\n"
	foundation, err := BuildQAFoundation(manifest, input.GovernedInputFingerprint, input.ImplementationFingerprint, input.ReviewFingerprint, nil, input.Settings.Budgets, "# Review\nCurrent.")
	if err != nil {
		t.Fatal(err)
	}
	input.Foundation = &foundation
	qaMap, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	shard := qaMap.Shards[0]
	service := NewService(t.TempDir()).WithQASettings(input.Settings)
	req, err := service.QAInvestigatorRequest(qaMap, shard, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"read", "list", "search", "glob"} {
		if req.Policy.Tools[tool] != "deny" {
			t.Fatalf("pack-complete tool %s = %q", tool, req.Policy.Tools[tool])
		}
	}
	if req.Cache.BreakpointBytes <= 0 || !validFingerprint(req.Cache.PrefixDigest) || !strings.Contains(req.Prompt[:req.Cache.BreakpointBytes], foundation.ID) {
		t.Fatalf("cache directive does not cover the foundation: %+v", req.Cache)
	}
	projectedPath := append(append([]string(nil), shard.ChangedPaths...), shard.ContextPaths...)[0]
	for i := range qaMap.Foundation.Blocks {
		if containsQAString(qaMap.Foundation.Blocks[i].RelatedPaths, projectedPath) {
			qaMap.Foundation.Blocks[i].OmittedBytes = 17
			break
		}
	}
	request, err := service.QAInvestigatorRequest(qaMap, shard, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if request.Policy.Tools["read"] != "allow" {
		t.Fatalf("read policy = %q, want allow for an incomplete bounded block", request.Policy.Tools["read"])
	}
	if !strings.Contains(request.Prompt, "omits 17 bytes") {
		t.Fatal("prompt does not declare the bounded block omission")
	}
}

func TestInvestigatorTheoryMaximumIsInclusive(t *testing.T) {
	budgets := DefaultQABudgets()
	output := qaInvestigatorOutput{Theories: make([]qaInvestigatorTheory, budgets.TheoriesPerShard), Context: make([]QAContextRequest, budgets.ContextExpansions), Checks: make([]QAApprovedCheckRef, budgets.CommandsPerAttempt)}
	if !withinQAInvestigatorBudgets(output, budgets) {
		t.Fatal("exactly the configured maximum must be accepted")
	}
	output.Theories = append(output.Theories, qaInvestigatorTheory{})
	if withinQAInvestigatorBudgets(output, budgets) {
		t.Fatal("one theory above the configured maximum must be rejected")
	}
}

func TestFoundationFingerprintChangesSemanticAttemptIdentity(t *testing.T) {
	input := qaMapInputFixture()
	first := &QAFoundation{Fingerprint: strings.Repeat("a", 64)}
	second := &QAFoundation{Fingerprint: strings.Repeat("b", 64)}
	firstIdentity := QASemanticIdentity{GovernedInputFingerprint: input.GovernedInputFingerprint, ImplementationFingerprint: input.ImplementationFingerprint, ReviewFingerprint: input.ReviewFingerprint, PolicyFingerprint: input.PolicyFingerprint, FoundationFingerprint: first.Fingerprint, ChangedPaths: input.ChangedPaths}
	secondIdentity := firstIdentity
	secondIdentity.FoundationFingerprint = second.Fingerprint
	firstID, err := NewQASemanticAttemptID(input.Project, input.Sprint, firstIdentity)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := NewQASemanticAttemptID(input.Project, input.Sprint, secondIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if firstID == secondID {
		t.Fatal("foundation changes must produce a distinct semantic attempt")
	}
}

func TestDiffFoundationKeepsEveryHunk(t *testing.T) {
	padding := strings.Repeat(" unchanged\n", 900)
	patch := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old-first\n+new-first\n" + padding + "@@ -1000 +1000 @@\n-old-last\n+new-last\n"
	blocks := qaDiffBlocks(patch)
	if len(blocks) != 2 {
		t.Fatalf("diff blocks = %d, want 2 exact hunks", len(blocks))
	}
	if !strings.Contains(blocks[0].Content, "+new-first") || !strings.Contains(blocks[1].Content, "+new-last") {
		t.Fatalf("diff hunks were not preserved: %+v", blocks)
	}
}
