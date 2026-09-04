package sprint

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

func TestRepairAdmissionAllowsEligibleIssueFromBlockedCurrentQA(t *testing.T) {
	state := QAState{
		Phase:               QAPhaseBlocked,
		Freshness:           QAFreshness{Current: true},
		CurrentAttemptID:    "qa-v1-attempt-current",
		CanonicalAssessment: AssessmentBlocked,
	}
	if !repairableQAState(state) {
		t.Fatal("blocked current QA with adjudicated issues must reach per-issue repair validation")
	}
	state.CurrentAttemptID = ""
	if repairableQAState(state) {
		t.Fatal("blocked QA without a current attempt must remain ineligible")
	}
	state.CurrentAttemptID = "qa-v1-attempt-current"
	state.CanonicalAssessment = AssessmentIncomplete
	if repairableQAState(state) {
		t.Fatal("incomplete QA must remain ineligible")
	}
}

func TestRepairAllowedPathsUsesImmutableShardForAuthoredTestIssue(t *testing.T) {
	paths, err := repairAllowedPaths(
		QAIssue{Location: "internal/sprint/qa_investigator_example_test.go"},
		[]QAEvidenceRecord{{ChangedPaths: []string{"internal/sprint/qa_investigator_example_test.go"}}},
		[]QAEvidencePlan{{ShardID: "shard-a"}},
		QAMap{Shards: []QAShard{{ID: "shard-a", ChangedPaths: []string{"internal/sprint/qa_repair_state.go"}}, {ID: "shard-b", ChangedPaths: []string{"internal/app/unrelated.go"}}}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(paths, []string{"internal/sprint/qa_repair_state.go"}) {
		t.Fatalf("allowed paths = %v", paths)
	}
}

func TestRegressionCandidateRepairPacketRequiresSignedCoverage(t *testing.T) {
	packet := repairPacketFixture(t)
	packet.Issue.RegressionCandidate = true
	if _, err := FinalizeRepairPacket(packet); err == nil || !strings.Contains(err.Error(), "missing issue evidence coverage") {
		t.Fatalf("regression candidate without coverage was accepted: %v", err)
	}
}

func TestFreezeRepairChecksPreservesAndRecoversWorkingDirectory(t *testing.T) {
	runID := "repair-v1-run-111111111111111111111111"
	planID := "qa-v2-plan-222222222222222222222222"
	base := QAEvidencePlan{
		ID: planID, CheckID: "qa-v2-test-333333333333333333333333",
		ApprovedPaths: []string{"internal/sprint/qa_investigator_example_test.go"},
		Executable:    "go", Args: []string{"test", "."}, Timeout: time.Minute,
		OutputLimit: 1024, RefutationCondition: "test passes",
	}
	evidence := []QAEvidenceRecord{{PlanID: planID, Outcome: QAEvidenceFail}}
	flow := FlowState{Smoke: &SmokeStageState{SmokeFingerprint: strings.Repeat("4", 64)}}
	checks, exact, err := freezeRepairChecks(runID, []QAEvidencePlan{base}, evidence, QAMap{}, flow, DefaultRepairBudgets())
	if err != nil {
		t.Fatal(err)
	}
	if exact.Workdir != "internal/sprint" || checks[0].Workdir != "internal/sprint" {
		t.Fatalf("legacy authored test workdir was not recovered: exact=%q check=%q", exact.Workdir, checks[0].Workdir)
	}
	base.WorkingDirectory = "internal/custom"
	checks, exact, err = freezeRepairChecks(runID, []QAEvidencePlan{base}, evidence, QAMap{}, flow, DefaultRepairBudgets())
	if err != nil {
		t.Fatal(err)
	}
	if exact.Workdir != "internal/custom" || checks[0].Workdir != "internal/custom" {
		t.Fatalf("frozen workdir was not preserved: exact=%q check=%q", exact.Workdir, checks[0].Workdir)
	}
}

func TestRepairCheckEnvironmentProvidesPrivateGoRuntimeAndCleansIt(t *testing.T) {
	target := filepath.Join(t.TempDir(), "target")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	environment, cleanup, err := repairCheckEnvironment(target, []string{"PATH"})
	if err != nil {
		t.Fatal(err)
	}
	runtimeRoot := filepath.Dir(environment["GOCACHE"])
	for _, name := range []string{"HOME", "GOCACHE", "GOPATH", "GOTMPDIR", "TMPDIR"} {
		if environment[name] == "" {
			t.Fatalf("private runtime omitted %s", name)
		}
	}
	if inside(target, runtimeRoot) {
		t.Fatalf("repair runtime %q was created inside the checked target", runtimeRoot)
	}
	readOnlyModule := filepath.Join(environment["GOPATH"], "pkg", "mod", "example")
	if err := os.MkdirAll(readOnlyModule, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(readOnlyModule, "cached.go"), []byte("package example\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(readOnlyModule, 0o500); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(runtimeRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("private runtime was not removed: %v", err)
	}
}

func TestRepairPacketDeterminismAndAuthorityValidation(t *testing.T) {
	packet := repairPacketFixture(t)
	first, err := FinalizeRepairPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FinalizeRepairPacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	if first.PacketDigest != second.PacketDigest || !reflect.DeepEqual(first, second) {
		t.Fatalf("packet finalization is not deterministic\nfirst=%+v\nsecond=%+v", first, second)
	}

	mutations := []struct {
		name string
		edit func(*RepairIssuePacket)
	}{
		{"ineligible", func(p *RepairIssuePacket) { p.Issue.RepairEligible = false }},
		{"cross-group", func(p *RepairIssuePacket) { p.Issue.RootCauseGroupID = strings.Repeat("x", 24) }},
		{"no reproducer", func(p *RepairIssuePacket) { p.ExactReproducer.Executable = "" }},
		{"protected path", func(p *RepairIssuePacket) { p.AllowedPaths = []string{"internal/sprint/qa_repair_test.go"} }},
		{"changed target", func(p *RepairIssuePacket) { p.Target.Fingerprint = strings.Repeat("b", 64) }},
		{"changed digest", func(p *RepairIssuePacket) { p.PacketDigest = strings.Repeat("b", 64) }},
	}
	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			changed := first
			test.edit(&changed)
			if err := ValidateRepairPacket(changed); err == nil {
				t.Fatal("changed packet passed validation")
			}
		})
	}
}

func TestRepairProtectedPathClassifier(t *testing.T) {
	tests := map[string]RepairPathClass{
		"internal/sprint/qa_repair.go":                        RepairPathProduction,
		"internal/sprint/qa_repair_test.go":                   RepairPathTestAsset,
		"tests/repair.ts":                                     RepairPathTestAsset,
		"projects/a/sprints/01/plan.md":                       RepairPathGovernedInput,
		"verification/state.json":                             RepairPathGeneratedEvidence,
		".ultra/config.json":                                  RepairPathWorkspaceState,
		".git/hooks/pre-commit":                               RepairPathRepositoryControl,
		".gitignore":                                          RepairPathRepositoryControl,
		"ultraplan.yml":                                       RepairPathConfiguration,
		"docs/recovery.md":                                    RepairPathNonProduction,
		"docs/recovery/cycle.md":                              RepairPathNonProduction,
		"verification/attempts/a/repairs/b/issue-packet.json": RepairPathGeneratedEvidence,
		"README.md":                                           RepairPathNonProduction,
		"internal/plans/plan.md":                              RepairPathGovernedInput,
		"../escape.go":                                        RepairPathUnsafe,
		"/absolute.go":                                        RepairPathUnsafe,
	}
	for path, want := range tests {
		if got := ClassifyRepairPath(path); got != want {
			t.Errorf("ClassifyRepairPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestRepairBudgetsAreLowerOnly(t *testing.T) {
	defaults := DefaultRepairBudgets()
	if err := ValidateRepairBudgets(defaults); err != nil {
		t.Fatal(err)
	}
	lower := defaults
	lower.MaxCycles = 2
	lower.MaxMutationCycles = 2
	if err := ValidateLowerRepairBudgets(lower, defaults); err != nil {
		t.Fatalf("lower budget rejected: %v", err)
	}
	higher := defaults
	higher.MaxCycles++
	higher.MaxMutationCycles++
	if err := ValidateLowerRepairBudgets(higher, defaults); err == nil {
		t.Fatal("increased budget accepted")
	}
	invalid := defaults
	invalid.MaxCycles = 0
	if err := ValidateRepairBudgets(invalid); err == nil {
		t.Fatal("zero budget accepted")
	}
}

func TestDeriveRepairOutcomeClosedVocabulary(t *testing.T) {
	tests := []struct {
		name  string
		facts RepairOutcomeFacts
		want  RepairOutcome
	}{
		{"verified", RepairOutcomeFacts{Mode: RepairModeManual, ExactIssueRemoved: true, AllRequiredPassed: true, CleanupComplete: true, TargetCurrent: true}, RepairOutcomeVerified},
		{"findings", RepairOutcomeFacts{Mode: RepairModeManual, ExactIssueRemoved: true, AllRequiredPassed: true, CleanupComplete: true, TargetCurrent: true, OnlyNonBlocking: true}, RepairOutcomeVerifiedWithFindings},
		{"failed", RepairOutcomeFacts{Mode: RepairModeManual, IssueStillReproduces: true}, RepairOutcomeFailed},
		{"blocked", RepairOutcomeFacts{Mode: RepairModeManual, PrerequisiteMissing: true}, RepairOutcomeBlocked},
		{"escalated", RepairOutcomeFacts{Mode: RepairModeManual, UnsafeOrUncertain: true}, RepairOutcomeEscalated},
		{"stalled", RepairOutcomeFacts{Mode: RepairModeAutomatic, Stagnated: true}, RepairOutcomeStalled},
		{"manual never stalled", RepairOutcomeFacts{Mode: RepairModeManual, Stagnated: true}, RepairOutcomeBlocked},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DeriveRepairOutcome(test.facts)
			if err != nil || got != test.want {
				t.Fatalf("outcome = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestRepairProgressUsesProductFacts(t *testing.T) {
	if RepairMadeProgress(RepairProgressFact{IssueCountBefore: 2, IssueCountAfter: 2}) {
		t.Fatal("unchanged issue set counted as progress")
	}
	for _, fact := range []RepairProgressFact{
		{ExactFailureRemoved: true},
		{IssueCountBefore: 2, IssueCountAfter: 1},
		{SeverityBefore: "high", SeverityAfter: "medium"},
	} {
		if !RepairMadeProgress(fact) {
			t.Fatalf("fact did not count as progress: %+v", fact)
		}
	}
}

func TestApplyRepairFilesChecksPreimageScopeAndCompensates(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"internal/a.go", "internal/b.go"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	expected := map[string]string{"internal/a.go": hashBytes([]byte("before\n"))}
	operations, changed, err := applyRepairFiles(root, map[string][]byte{"internal/a.go": []byte("after\n")}, expected, 1, 100)
	if err != nil || changed == 0 || len(operations) != 1 || !operations[0].Applied {
		t.Fatalf("apply = %+v, bytes=%d, err=%v", operations, changed, err)
	}
	data, _ := os.ReadFile(filepath.Join(root, "internal/a.go"))
	if string(data) != "after\n" {
		t.Fatalf("applied bytes = %q", data)
	}
	if _, _, err := applyRepairFiles(root, map[string][]byte{"internal/b_test.go": []byte("weaken\n")}, map[string]string{"internal/b_test.go": strings.Repeat("a", 64)}, 1, 100); err == nil {
		t.Fatal("test change accepted")
	}
	if _, _, err := applyRepairFiles(root, map[string][]byte{"internal/a.go": []byte("again\n")}, expected, 1, 100); err == nil {
		t.Fatal("stale preimage accepted")
	}
}

func TestApplyRepairFilesRejectsInvalidUTF8WithoutMutation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "internal", "a.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	before := []byte("before\n")
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatal(err)
	}
	invalid := []byte{0xc3, 0x28, 0xa0, 0xa1}
	operations, _, err := applyRepairFiles(root, map[string][]byte{"internal/a.go": invalid}, map[string]string{"internal/a.go": hashBytes(before)}, 1, 1024)
	if err == nil || len(operations) != 0 {
		t.Fatalf("invalid UTF-8 apply = %+v, %v", operations, err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(after, before) {
		t.Fatalf("target changed: %q, %v", after, readErr)
	}
}

func TestPairedTestAndImplementationApplyCompensatesAtomically(t *testing.T) {
	root := t.TempDir()
	implementation := filepath.Join(root, "internal", "a.go")
	testPath := filepath.Join(root, "internal", "a_test.go")
	if err := os.MkdirAll(filepath.Dir(implementation), 0o755); err != nil {
		t.Fatal(err)
	}
	before := []byte("package internal\nfunc value() int { return 1 }\n")
	if err := os.WriteFile(implementation, before, 0o644); err != nil {
		t.Fatal(err)
	}
	replacements := map[string][]byte{
		"internal/a.go":      []byte("package internal\nfunc value() int { return 2 }\n"),
		"internal/a_test.go": []byte("package internal\n// immutable regression test\n"),
	}
	preimages := map[string]string{"internal/a.go": hashBytes(before), "internal/a_test.go": repairMissingPreimageDigest}
	operations, _, err := applyRepairFilesJournaled(root, replacements, preimages, 2, 4096, func(current []RepairApplyOperation) error {
		if len(current) == 2 && current[1].Applied {
			return errors.New("injected journal failure")
		}
		return nil
	})
	if err == nil || !repairApplyCompensated(operations) {
		t.Fatalf("paired compensation = %+v, %v", operations, err)
	}
	if current, readErr := os.ReadFile(implementation); readErr != nil || !bytes.Equal(current, before) {
		t.Fatalf("implementation was not restored: %q, %v", current, readErr)
	}
	if _, statErr := os.Lstat(testPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("created test survived compensation: %v", statErr)
	}
	operations, _, err = applyRepairFiles(root, replacements, preimages, 2, 4096)
	if err != nil || len(operations) != 2 || !operations[0].Applied || !operations[1].Applied {
		t.Fatalf("paired apply = %+v, %v", operations, err)
	}
}

func TestRepairProposalSplitsFrozenTestAndImplementationPatches(t *testing.T) {
	var proposal strings.Builder
	writeWholeFilePatch(&proposal, "internal/a_test.go", nil, []byte("package internal\n"))
	writeWholeFilePatch(&proposal, "internal/a.go", []byte("before\n"), []byte("after\n"))
	testPatch, implementationPatch := splitRepairProposalPatches([]byte(proposal.String()), []string{"internal/a_test.go"})
	if !bytes.Contains(testPatch, []byte("internal/a_test.go")) || bytes.Contains(testPatch, []byte("internal/a.go\n")) {
		t.Fatalf("test patch = %q", testPatch)
	}
	if !bytes.Contains(implementationPatch, []byte("internal/a.go")) || bytes.Contains(implementationPatch, []byte("internal/a_test.go")) {
		t.Fatalf("implementation patch = %q", implementationPatch)
	}
}

func TestRepairApplyRejectsHardLinks(t *testing.T) {
	root := t.TempDir()
	original := filepath.Join(root, "internal", "a.go")
	if err := os.MkdirAll(filepath.Dir(original), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(original, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(original, filepath.Join(root, "internal", "alias.go")); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if _, _, err := applyRepairFiles(root, map[string][]byte{"internal/a.go": []byte("after\n")}, map[string]string{"internal/a.go": hashBytes([]byte("before\n"))}, 1, 100); err == nil {
		t.Fatal("hard-linked production file was accepted")
	}
}

func TestRepairApplyJournalRetainsDigestBoundPrivatePreimages(t *testing.T) {
	packet := repairPacketFixture(t)
	root := t.TempDir()
	sp := Sprint{Project: packet.Project, Slug: packet.Sprint, Path: filepath.Join(root, "projects", packet.Project, "sprints", packet.Sprint)}
	target := filepath.Join(root, "target")
	path := filepath.Join(target, "internal", "a.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	preimage := []byte("before\n")
	if err := os.WriteFile(path, preimage, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sp.Path, 0o700); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	state := RepairState{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, Mode: RepairModeManual, Phase: RepairPhaseApplying, Freshness: RepairFreshness{Current: true}, Run: QARunCorrelation{Lifecycle: QARunActive, RunID: "operation", OperationalAttemptID: "attempt", FencingGeneration: 1}, Packet: &QAArtifactRef{Path: "packet", Digest: strings.Repeat("1", 64)}, Confirmation: &QAArtifactRef{Path: "confirmation", Digest: strings.Repeat("2", 64)}, CurrentCycle: 1, EarliestCycle: 1, Deadline: now.Add(time.Hour), NextAction: "Apply retained proposal.", UpdatedAt: now}
	flow := NewFlowState(sp, emptyPlanningStageStates(sp), now)
	token := QAWriterToken{RunID: "operation", OperationalAttemptID: "attempt", FencingGeneration: 1}
	store := NewQAStore(root, sp).WithWriterFence(func(got QAWriterToken) error {
		if got != token {
			return os.ErrPermission
		}
		return nil
	})
	journal, err := store.StageRepairApplyJournal(state, flow, 1, target, map[string][]byte{"internal/a.go": []byte("after\n")}, map[string]string{"internal/a.go": hashBytes(preimage)}, token)
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != "planned" || len(journal.Operations) != 1 || journal.Operations[0].PreimagePath == "" {
		t.Fatalf("journal=%+v", journal)
	}
	loaded, err := store.LoadRepairApplyJournal(packet.QAAttemptID, packet.RepairRunID, 1)
	if err != nil {
		t.Fatal(err)
	}
	retained, err := store.loadRepairPreimage(loaded.Operations[0])
	if err != nil || !reflect.DeepEqual(retained, preimage) {
		t.Fatalf("preimage=%q err=%v", retained, err)
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(loaded.Operations[0].PreimagePath)))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("preimage mode=%v", info.Mode())
	}
}

func TestRepairReverificationRunsRepairedTargetSmokeWithoutSecondReview(t *testing.T) {
	packet := repairPacketFixture(t)
	for i := range packet.Checks {
		packet.Checks[i].Executable = "true"
		packet.Checks[i].Args = nil
	}
	packet.Checks[len(packet.Checks)-1].Executable = "@product"
	target := t.TempDir()
	flow := FlowState{
		Review: &ReviewStageState{Fingerprint: packet.ReviewFingerprint},
		Smoke:  &SmokeStageState{SmokeFingerprint: packet.SmokeFingerprint},
	}
	verification, exactRemoved, _, complete := NewService(t.TempDir()).runRepairReverification(context.Background(), packet, target, flow, 1, false, false, nil)
	if !exactRemoved {
		t.Fatal("passing exact reproducer was not recorded")
	}
	if complete {
		t.Fatal("smoke unexpectedly passed without a sprint smoke harness")
	}
	smoke := verification.Gates[len(verification.Gates)-1]
	if smoke.Status != RepairGateFailed || !strings.Contains(smoke.Reason, "repaired-target") {
		t.Fatalf("smoke gate=%+v", smoke)
	}
	if len(verification.Gates) != len(RepairGateOrder()) {
		t.Fatalf("gate count=%d", len(verification.Gates))
	}
}

func TestIntermediateCampaignRepairDefersOnlyGlobalSmoke(t *testing.T) {
	packet := repairPacketFixture(t)
	for i := range packet.Checks {
		packet.Checks[i].Executable = "true"
		packet.Checks[i].Args = nil
	}
	packet.Checks[len(packet.Checks)-1].Executable = "@product"
	verification, exactRemoved, issueChecksPassed, completeLadder := NewService(t.TempDir()).runRepairReverification(context.Background(), packet, t.TempDir(), FlowState{}, 1, true, true, nil)
	if !exactRemoved || !issueChecksPassed || completeLadder {
		t.Fatalf("intermediate facts exact=%t issue_checks=%t ladder=%t", exactRemoved, issueChecksPassed, completeLadder)
	}
	for i, gate := range verification.Gates {
		if i == len(verification.Gates)-1 {
			if gate.Gate != RepairGateContainingSmoke || gate.Status != RepairGateDeferred {
				t.Fatalf("final gate = %+v", gate)
			}
		} else if gate.Status != RepairGatePassed {
			t.Fatalf("issue-scoped gate %d = %+v", i, gate)
		}
	}
	if err := ValidateRepairReverification(verification); err != nil {
		t.Fatalf("campaign deferral is not publishable: %v", err)
	}
}

func TestEveryRepairBudgetFieldRejectsAConfiguredRaise(t *testing.T) {
	ceiling := DefaultRepairBudgets()
	typ := reflect.TypeOf(ceiling)
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			raised := ceiling
			value := reflect.ValueOf(&raised).Elem().FieldByName(field.Name)
			value.SetInt(value.Int() + 1)
			if err := ValidateLowerRepairBudgets(raised, ceiling); err == nil {
				t.Fatal("raised field accepted")
			}
		})
	}
}

func TestDeriveRepairOutcomeCoversEveryStopReasonAndUncertainFact(t *testing.T) {
	escalated := []RepairStopReason{RepairStopCleanupUncertain, RepairStopScopeGrowth, RepairStopSeverityGrowth, RepairStopTargetDrift, RepairStopGovernedDrift, RepairStopDesignDecision, RepairStopContradiction, RepairStopUnsupportedChange, RepairStopUncertainEvidence, RepairStopUnknownSchema}
	for _, stop := range escalated {
		got, err := DeriveRepairOutcome(RepairOutcomeFacts{Mode: RepairModeManual, StopReason: stop})
		if err != nil || got != RepairOutcomeEscalated {
			t.Errorf("stop %s = %s, %v", stop, got, err)
		}
	}
	for _, stop := range []RepairStopReason{RepairStopPrerequisite, RepairStopCancellation} {
		got, _ := DeriveRepairOutcome(RepairOutcomeFacts{Mode: RepairModeManual, StopReason: stop})
		if got != RepairOutcomeBlocked {
			t.Errorf("stop %s = %s", stop, got)
		}
	}
	for _, stop := range []RepairStopReason{RepairStopStagnation, RepairStopRepeatedPatch, RepairStopRepeatedTarget, RepairStopCycleLimit, RepairStopReopeningLimit} {
		automatic, _ := DeriveRepairOutcome(RepairOutcomeFacts{Mode: RepairModeAutomatic, StopReason: stop})
		manual, _ := DeriveRepairOutcome(RepairOutcomeFacts{Mode: RepairModeManual, StopReason: stop})
		if automatic != RepairOutcomeStalled || manual == RepairOutcomeStalled {
			t.Errorf("stop %s automatic=%s manual=%s", stop, automatic, manual)
		}
	}
	for name, facts := range map[string]RepairOutcomeFacts{
		"cleanup":  {Mode: RepairModeManual, ExactIssueRemoved: true, AllRequiredPassed: true, TargetCurrent: true},
		"target":   {Mode: RepairModeManual, ExactIssueRemoved: true, AllRequiredPassed: true, CleanupComplete: true},
		"required": {Mode: RepairModeManual, RequiredCheckFailed: true},
	} {
		got, _ := DeriveRepairOutcome(facts)
		if name == "required" && got != RepairOutcomeFailed || name != "required" && got != RepairOutcomeBlocked {
			t.Errorf("%s = %s", name, got)
		}
	}
}

func TestRepairCheckSequenceRequiresEveryAdjacentGate(t *testing.T) {
	packet := repairPacketFixture(t)
	packet.Checks = append(packet.Checks[:2:2], packet.Checks[3:]...)
	if err := validateRepairCheckSequence(packet.Checks, packet.Budgets); err == nil || !strings.Contains(err.Error(), "omit") {
		t.Fatalf("gap error = %v", err)
	}
}

func TestRepairProposalCarriesVersionedPromptIdentity(t *testing.T) {
	packet := repairPacketFixture(t)
	settings := QASettings{Runtime: StageRuntime{Model: "test/model", Variant: "high"}, Budgets: DefaultQABudgets()}
	request, err := NewService(t.TempDir()).WithQASettings(settings).repairProposalRequest(packet, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"prompt_id": repairProposalPromptID, "prompt_version": repairProposalPromptVersion, "prompt_checksum": hashBytes([]byte(repairProposalPromptBody))} {
		if request.Metadata[key] != want {
			t.Errorf("%s = %q, want %q", key, request.Metadata[key], want)
		}
	}
	if request.Policy.UnsupportedBehavior != "" {
		t.Fatalf("unsupported permission behavior = %q, want fail-closed default", request.Policy.UnsupportedBehavior)
	}
}

func TestRepairCampaignCreatesConfiguredWorkersAndMatchesFreshIssues(t *testing.T) {
	groupA, _ := NewQAV2ID("group", "alpha", "38-repair", "attempt", "a")
	groupB, _ := NewQAV2ID("group", "alpha", "38-repair", "attempt", "b")
	issues := []QAIssue{
		{ID: "issue-a", RootCauseGroupID: groupA, Title: "first", IssueClass: "logic", Location: "internal/a.go", RepairEligible: true},
		{ID: "issue-b", RootCauseGroupID: groupA, Title: "second", IssueClass: "logic", Location: "internal/a.go", RepairEligible: true},
		{ID: "issue-c", RootCauseGroupID: groupB, Title: "third", IssueClass: "api", Location: "internal/b.go", RepairEligible: true},
	}
	adjudication := QAAdjudication{ID: "adjudication", Issues: issues, Groups: []QARootCauseGroup{{ID: groupA, Claim: "shared cause"}, {ID: groupB, Claim: "other cause"}}, RepairGroups: []QARepairIssueGroup{{IssueIDs: []string{"issue-a", "issue-b"}}, {IssueIDs: []string{"issue-c"}}}}
	assignments, err := PlanRepairAssignments(adjudication, "grouped", 2)
	if err != nil {
		t.Fatal(err)
	}
	state, err := newRepairCampaignState(Sprint{Project: "alpha", Slug: "38-repair"}, adjudication, assignments, QASettings{RepairAssignmentMode: "grouped", IssuesPerRepairAgent: 2, RepairExecutionMode: "parallel"}, time.Unix(1, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Workers) != 2 || state.Total != 3 || len(state.Workers[0].Issues) != 2 {
		t.Fatalf("campaign state = %+v", state)
	}
	if state.ExecutionMode != "parallel" {
		t.Fatalf("campaign execution mode = %q", state.ExecutionMode)
	}
	if err := ValidateRepairCampaignState(state); err != nil {
		t.Fatalf("new campaign state is not publishable: %v; state=%+v", err, state)
	}
	freshGroup, _ := NewQAV2ID("group", "alpha", "38-repair", "fresh", "a")
	freshIssue := QAIssue{ID: "fresh-issue", RootCauseGroupID: freshGroup, Title: "renamed", IssueClass: "logic", Location: "internal/a.go", RepairEligible: true}
	matched := matchCampaignIssue(QAAdjudication{Issues: []QAIssue{freshIssue}, Groups: []QARootCauseGroup{{ID: freshGroup, Claim: "shared cause"}}}, state.Workers[0].Issues[0])
	if matched == nil || matched.ID != freshIssue.ID {
		t.Fatalf("fresh issue match = %+v", matched)
	}
}

func TestRepairCampaignStateIsDurablyReplaceable(t *testing.T) {
	root := t.TempDir()
	sp := Sprint{Project: "alpha", Slug: "38-repair", Path: filepath.Join(root, "projects", "alpha", "sprints", "38-repair")}
	token := QAWriterToken{RunID: "operation", OperationalAttemptID: "attempt", FencingGeneration: 1}
	store := NewQAStore(root, sp).WithWriterFence(func(got QAWriterToken) error {
		if got != token {
			return os.ErrPermission
		}
		return nil
	})
	now := time.Unix(1, 0).UTC()
	state := RepairCampaignState{SchemaVersion: repairCampaignSchemaVersion, ID: "repair-campaign-v1-aaaaaaaaaaaaaaaaaaaaaaaa", Project: sp.Project, Sprint: sp.Slug, Mode: "grouped", PerAgent: 2, Status: "running", Workers: []RepairCampaignWorker{{Number: 1, Issues: []RepairCampaignIssue{{OriginalIssueID: "issue-a", Title: "bug", IssueClass: "logic", RootCauseClaim: "cause", Status: "queued"}}}}, Total: 1, StartedAt: now, UpdatedAt: now}
	if err := store.publishRepairCampaign(state, token); err != nil {
		t.Fatal(err)
	}
	completed := now.Add(time.Minute)
	state.Status, state.Completed, state.UpdatedAt, state.CompletedAt = "completed", 1, completed, &completed
	state.Workers[0].Issues[0].Status = "verified"
	if err := store.publishRepairCampaign(state, token); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRepairCampaign()
	if err != nil || loaded.Status != "completed" || loaded.Completed != 1 {
		t.Fatalf("loaded campaign = %+v, %v", loaded, err)
	}
}

func TestParallelCampaignProposalRequiresCurrentAllowedPreimages(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "internal", "repair.go")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	before := []byte("package internal\n")
	if err := os.WriteFile(path, before, 0o600); err != nil {
		t.Fatal(err)
	}
	packet := RepairIssuePacket{AllowedPaths: []string{"internal/repair.go"}, Budgets: DefaultRepairBudgets()}
	prepared := &campaignPreparedProposal{
		Proposal:     []byte("patch"),
		Replacements: map[string][]byte{"internal/repair.go": []byte("package internal\n// fixed\n")},
		Preimages:    map[string]string{"internal/repair.go": hashBytes(before)},
		ChangedPaths: []string{"internal/repair.go"},
		ChangedBytes: int64(len(before) + len("package internal\n// fixed\n")),
	}
	if !campaignPreparedProposalCurrent(target, packet, prepared) {
		t.Fatal("current allowed parallel proposal was rejected")
	}
	if err := os.WriteFile(path, []byte("package internal\n// earlier repair\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if campaignPreparedProposalCurrent(target, packet, prepared) {
		t.Fatal("stale parallel proposal preimage was accepted")
	}
	prepared.ChangedPaths = []string{"internal/other.go"}
	if campaignPreparedProposalCurrent(target, packet, prepared) {
		t.Fatal("out-of-scope parallel proposal was accepted")
	}
}

func TestRepairEvidenceUsesGitAndTreeIdentitiesForTheirOwnPurposes(t *testing.T) {
	qaMap := QAMap{ImplementationFingerprint: strings.Repeat("a", 64)}
	record := QAEvidenceRecord{
		Outcome: QAEvidenceFail, Contained: true, Repeatable: true,
		Cleanup:                   QACleanupFacts{Complete: true},
		ImplementationFingerprint: qaMap.ImplementationFingerprint,
		TargetIdentityBefore:      strings.Repeat("b", 64),
		TargetIdentityAfter:       strings.Repeat("b", 64),
	}
	if !repairEvidenceMatchesMap(record, qaMap) {
		t.Fatal("unchanged real QA tree identity was rejected because it differs from the Git implementation fingerprint")
	}
	record.TargetIdentityAfter = strings.Repeat("c", 64)
	if repairEvidenceMatchesMap(record, qaMap) {
		t.Fatal("target tree drift was admitted")
	}
	record.TargetIdentityAfter = record.TargetIdentityBefore
	record.ImplementationFingerprint = strings.Repeat("d", 64)
	if repairEvidenceMatchesMap(record, qaMap) {
		t.Fatal("stale Git implementation fingerprint was admitted")
	}
}

type repairFailingRunner struct{}

func (repairFailingRunner) Run(context.Context, pprocess.Request) (pprocess.Result, error) {
	return pprocess.Result{ExitCode: -1, CleanupComplete: true}, os.ErrDeadlineExceeded
}

type repairProvisionalRunner struct {
	exitCode int
	calls    int
}

func (runner *repairProvisionalRunner) Run(context.Context, pprocess.Request) (pprocess.Result, error) {
	runner.calls++
	return pprocess.Result{ExitCode: runner.exitCode, CleanupComplete: true}, nil
}

func TestRepairProvisionalChecksRunFrozenCommandsOnceAndFailClosed(t *testing.T) {
	packet := repairPacketFixture(t)
	target := t.TempDir()
	passing := &repairProvisionalRunner{}
	results, ok := NewService(t.TempDir()).WithProcessRunner(passing).runRepairProvisionalChecks(context.Background(), packet, target)
	if !ok || len(results) != 1 || passing.calls != 1 || results[0].Status != RepairGatePassed {
		t.Fatalf("passing provisional checks = %+v ok=%v calls=%d", results, ok, passing.calls)
	}
	failing := &repairProvisionalRunner{exitCode: 1}
	results, ok = NewService(t.TempDir()).WithProcessRunner(failing).runRepairProvisionalChecks(context.Background(), packet, target)
	if ok || len(results) != 1 || failing.calls != 1 || results[0].Status != RepairGateFailed {
		t.Fatalf("failing provisional checks = %+v ok=%v calls=%d", results, ok, failing.calls)
	}
}

func TestRepairReverificationRetainsBoundedRunnerDiagnostic(t *testing.T) {
	packet := repairPacketFixture(t)
	for i := range packet.Checks {
		packet.Checks[i].Executable = "go"
	}
	verification, _, _, _ := NewService(t.TempDir()).WithProcessRunner(repairFailingRunner{}).runRepairReverification(context.Background(), packet, t.TempDir(), FlowState{}, 1, false, false, nil)
	if verification.Gates[0].Diagnostic == "" || !strings.Contains(verification.Gates[0].Diagnostic, "timeout") {
		t.Fatalf("gate=%+v", verification.Gates[0])
	}
}

func TestRepairDiagnosticRedactsSecretsAndHostPaths(t *testing.T) {
	for _, value := range []string{"Bearer abc", "token=secret", "/home/operator/source", "/tmp/private"} {
		if got := boundRepairText(value, 512); got != "[redacted repair diagnostic]" {
			t.Errorf("%q => %q", value, got)
		}
	}
}

func TestJoinRepairDiagnosticsOmitsEmptyEntries(t *testing.T) {
	if got := joinRepairDiagnostics("", " ", "target drift"); got != "target drift" {
		t.Fatalf("diagnostic = %q", got)
	}
	if got := joinRepairDiagnostics("", " "); got != "" {
		t.Fatalf("empty diagnostic = %q", got)
	}
}

func TestRepairConfirmationRejectsEveryAuthorityMutation(t *testing.T) {
	packet, err := FinalizeRepairPacket(repairPacketFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	base, err := FinalizeRepairConfirmation(RepairConfirmation{Project: packet.Project, Sprint: packet.Sprint, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, PacketDigest: packet.PacketDigest, Target: packet.Target, Mode: packet.Mode, Budgets: packet.Budgets, GovernedInputFingerprint: packet.GovernedInputFingerprint, PolicyFingerprint: packet.PolicyFingerprint, OperationRunID: "operation", OperationalAttemptID: "attempt", FencingGeneration: 1, Confirmer: "operator", ConfirmedAt: time.Unix(200, 0)}, packet)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*RepairConfirmation){"mode": func(v *RepairConfirmation) { v.Mode = RepairModeAutomatic }, "opt_in": func(v *RepairConfirmation) { v.AutomaticOptIn = true }, "budget": func(v *RepairConfirmation) { v.Budgets.MaxFilesPerRun-- }, "target": func(v *RepairConfirmation) { v.Target.Fingerprint = strings.Repeat("b", 64) }, "governed": func(v *RepairConfirmation) { v.GovernedInputFingerprint = strings.Repeat("b", 64) }, "policy": func(v *RepairConfirmation) { v.PolicyFingerprint = strings.Repeat("b", 64) }, "run": func(v *RepairConfirmation) { v.OperationRunID = "other" }, "attempt": func(v *RepairConfirmation) { v.OperationalAttemptID = "other" }, "fence": func(v *RepairConfirmation) { v.FencingGeneration++ }, "confirmer": func(v *RepairConfirmation) { v.Confirmer = "other" }, "time": func(v *RepairConfirmation) { v.ConfirmedAt = v.ConfirmedAt.Add(time.Second) }}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := base
			mutate(&changed)
			if err := ValidateRepairConfirmation(changed, packet); err == nil {
				t.Fatal("mutation accepted")
			}
		})
	}
}

func TestQualifyManualRepairProofRejectsEveryWeakOrStaleFact(t *testing.T) {
	packet, err := FinalizeRepairPacket(repairPacketFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	result := RepairResult{SchemaVersion: QARepairSchemaVersion, Project: packet.Project, Sprint: packet.Sprint, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, Mode: RepairModeManual, Outcome: RepairOutcomeVerified, Target: packet.Target, CleanupComplete: true, ProductionApplied: true, CompleteLadder: true}
	protocol, runtime := strings.Repeat("7", 64), strings.Repeat("8", 64)
	proof := ManualRepairProof{SchemaVersion: QARepairSchemaVersion, Project: packet.Project, Sprint: packet.Sprint, RepairRunID: packet.RepairRunID, PacketDigest: packet.PacketDigest, ResultDigest: strings.Repeat("9", 64), Outcome: result.Outcome, Target: result.Target, ProtocolFingerprint: protocol, ImplementationFingerprint: result.Target.Fingerprint, PolicyFingerprint: packet.PolicyFingerprint, IsolationFingerprint: packet.IsolationFingerprint, GovernedInputFingerprint: packet.GovernedInputFingerprint, RuntimeFingerprint: runtime, CleanupComplete: true, ProductionApplied: true, CompleteLadder: true, PublishedAt: time.Unix(300, 0)}
	if err := QualifyManualRepairProof(proof, packet, result, protocol, runtime); err != nil {
		t.Fatalf("valid proof: %v", err)
	}
	mutations := map[string]func(*ManualRepairProof){"outcome": func(v *ManualRepairProof) { v.Outcome = RepairOutcomeFailed }, "cleanup": func(v *ManualRepairProof) { v.CleanupComplete = false }, "apply": func(v *ManualRepairProof) { v.ProductionApplied = false }, "ladder": func(v *ManualRepairProof) { v.CompleteLadder = false }, "implementation": func(v *ManualRepairProof) { v.ImplementationFingerprint = strings.Repeat("c", 64) }, "policy": func(v *ManualRepairProof) { v.PolicyFingerprint = strings.Repeat("a", 64) }, "isolation": func(v *ManualRepairProof) { v.IsolationFingerprint = strings.Repeat("a", 64) }, "governed": func(v *ManualRepairProof) { v.GovernedInputFingerprint = strings.Repeat("a", 64) }, "protocol": func(v *ManualRepairProof) { v.ProtocolFingerprint = strings.Repeat("a", 64) }, "runtime": func(v *ManualRepairProof) { v.RuntimeFingerprint = strings.Repeat("a", 64) }}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := proof
			mutate(&changed)
			if err := QualifyManualRepairProof(changed, packet, result, protocol, runtime); err == nil {
				t.Fatal("weak proof accepted")
			}
		})
	}
}

func TestPrepareRepairRejectsWhileTerminalizing(t *testing.T) {
	err := validateRepairPreparationBarrier(RepairState{Phase: RepairPhaseTerminalizing}, nil)
	typed, ok := AsQAError(err)
	if !ok || typed.Category != QAErrorConflict {
		t.Fatalf("barrier error = %v", err)
	}
	if err := validateRepairPreparationBarrier(RepairState{}, os.ErrNotExist); err != nil {
		t.Fatalf("missing prior state blocked: %v", err)
	}
}

func TestRecoverRepairRefusesEveryNonRecoverablePhase(t *testing.T) {
	recoverable := map[RepairPhase]bool{RepairPhaseProposing: true, RepairPhaseApplying: true, RepairPhaseReverifying: true, RepairPhaseCleaning: true, RepairPhaseInterrupted: true}
	for _, phase := range []RepairPhase{RepairPhasePrepared, RepairPhaseConfirmed, RepairPhaseProposing, RepairPhaseApplying, RepairPhaseReverifying, RepairPhaseCleaning, RepairPhaseTerminalizing, RepairPhaseTerminal, RepairPhaseInterrupted, RepairPhaseStale} {
		err := validateRepairRecoverablePhase(phase)
		if recoverable[phase] && err != nil {
			t.Errorf("%s rejected: %v", phase, err)
		}
		if !recoverable[phase] && err == nil {
			t.Errorf("%s accepted", phase)
		}
	}
}

func TestRepairRetainedCycleRecordsRejectCorruptionAndIdentitySwaps(t *testing.T) {
	packet := repairPacketFixture(t)
	now := time.Unix(500, 0).UTC()
	ref := &QAArtifactRef{Path: "projects/alpha/sprints/38-repair/verification/attempts/x/repairs/y/cycles/000001/scope.json", Digest: strings.Repeat("a", 64)}
	cycle := RepairCycle{SchemaVersion: QARepairSchemaVersion, RepairRunID: packet.RepairRunID, Number: 1, Scope: ref, StartedAt: now, CompletedAt: &now}
	if err := ValidateRepairCycle(cycle); err != nil {
		t.Fatalf("valid cycle rejected: %v", err)
	}
	for name, mutate := range map[string]func(*RepairCycle){
		"schema": func(v *RepairCycle) { v.SchemaVersion++ },
		"run":    func(v *RepairCycle) { v.RepairRunID = "repair-v1-run-invalid" },
		"number": func(v *RepairCycle) { v.Number = 0 },
		"time":   func(v *RepairCycle) { before := now.Add(-time.Second); v.StartedAt = now; v.CompletedAt = &before },
		"path":   func(v *RepairCycle) { v.Scope.Path = "../scope.json" },
		"digest": func(v *RepairCycle) { v.Scope.Digest = "short" },
	} {
		t.Run("cycle_"+name, func(t *testing.T) {
			changed := cycle
			copyRef := *cycle.Scope
			changed.Scope = &copyRef
			mutate(&changed)
			if err := ValidateRepairCycle(changed); err == nil {
				t.Fatal("corrupt cycle accepted")
			}
		})
	}

	scope := RepairScopeRecord{SchemaVersion: QARepairSchemaVersion, RepairRunID: packet.RepairRunID, Cycle: 1, Before: packet.Target, After: packet.Target, IntendedPaths: []string{"internal/a.go"}, ActualPaths: []string{"internal/a.go"}, ChangedBytes: 4, Enforced: true}
	if err := ValidateRepairScope(scope); err != nil {
		t.Fatalf("valid scope rejected: %v", err)
	}
	changedScope := scope
	changedScope.ActualPaths = []string{"internal/b.go"}
	if err := ValidateRepairScope(changedScope); err == nil {
		t.Fatal("enforced scope mismatch accepted")
	}
	changedScope = scope
	changedScope.ActualPaths = []string{"internal/z.go", "internal/a.go"}
	changedScope.Enforced = false
	if err := ValidateRepairScope(changedScope); err == nil {
		t.Fatal("unordered scope accepted")
	}

	cleanup := RepairCleanup{SchemaVersion: QARepairSchemaVersion, RepairRunID: packet.RepairRunID, Cycle: 1, ProcessTreeTerminated: true, WorkspaceRemoved: true, CompensationKnown: true, TargetCurrent: true, LeaseReleased: true, Complete: true}
	if err := ValidateRepairCleanup(cleanup); err != nil {
		t.Fatalf("valid cleanup rejected: %v", err)
	}
	cleanup.LeaseReleased = false
	if err := ValidateRepairCleanup(cleanup); err == nil {
		t.Fatal("cleanup claimed complete before lease release")
	}
}

func TestRepairObservabilityRejectsImpossibleCounters(t *testing.T) {
	if err := ValidateRepairConsumed(RepairConsumed{Commands: -1}); err == nil {
		t.Fatal("negative command count accepted")
	}
	now := time.Unix(600, 0).UTC()
	runtime := RepairRuntimeObservation{Provider: "openrouter", Model: "minimax/minimax-m3:free", StartedAt: now, CompletedAt: now.Add(time.Second), Duration: time.Second, DurationMS: 1000, RuntimeEvents: 3, RetainedEvents: 3, ObservedToolCalls: 1}
	if err := ValidateRepairRuntime(runtime); err != nil {
		t.Fatalf("valid runtime rejected: %v", err)
	}
	runtime.Usage.InputTokens = -1
	if err := ValidateRepairRuntime(runtime); err == nil {
		t.Fatal("negative token count accepted")
	}
	runtime.Usage.InputTokens = 0
	runtime.CompletedAt = now.Add(-time.Second)
	if err := ValidateRepairRuntime(runtime); err == nil {
		t.Fatal("backwards runtime timestamps accepted")
	}
}

func TestReconcileRepairJournalCoversNoWriteCompensationAndUnknownDrift(t *testing.T) {
	for _, test := range []struct {
		name              string
		current           string
		wantApplied       bool
		wantComplete      bool
		wantState         string
		wantFinalContents string
		wantRestored      bool
	}{
		{name: "no production write", current: "before\n", wantComplete: true, wantState: "compensated", wantFinalContents: "before\n"},
		{name: "exact postimage", current: "after\n", wantApplied: true, wantComplete: true, wantState: "compensated", wantFinalContents: "before\n", wantRestored: true},
		{name: "unknown drift", current: "third-party\n", wantComplete: false, wantState: "uncertain", wantFinalContents: "third-party\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			packet := repairPacketFixture(t)
			root := t.TempDir()
			sp := Sprint{Project: packet.Project, Slug: packet.Sprint, Path: filepath.Join(root, "projects", packet.Project, "sprints", packet.Sprint)}
			target := filepath.Join(root, "target")
			targetPath := filepath.Join(target, "internal", "a.go")
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(targetPath, []byte("before\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(sp.Path, 0o700); err != nil {
				t.Fatal(err)
			}
			now := time.Unix(700, 0).UTC()
			token := QAWriterToken{RunID: "operation", OperationalAttemptID: "attempt", FencingGeneration: 1}
			state := RepairState{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, Mode: RepairModeManual, Phase: RepairPhaseApplying, Freshness: RepairFreshness{Current: true}, Run: QARunCorrelation{Lifecycle: QARunActive, RunID: token.RunID, OperationalAttemptID: token.OperationalAttemptID, FencingGeneration: token.FencingGeneration}, Packet: &QAArtifactRef{Path: "packet", Digest: strings.Repeat("1", 64)}, Confirmation: &QAArtifactRef{Path: "confirmation", Digest: strings.Repeat("2", 64)}, CurrentCycle: 1, EarliestCycle: 1, Deadline: now.Add(time.Hour), NextAction: "Apply.", UpdatedAt: now}
			store := NewQAStore(root, sp).WithWriterFence(func(got QAWriterToken) error {
				if got != token {
					return os.ErrPermission
				}
				return nil
			})
			journal, err := store.StageRepairApplyJournal(state, NewFlowState(sp, emptyPlanningStageStates(sp), now), 1, target, map[string][]byte{"internal/a.go": []byte("after\n")}, map[string]string{"internal/a.go": hashBytes([]byte("before\n"))}, token)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(targetPath, []byte(test.current), 0o600); err != nil {
				t.Fatal(err)
			}
			applied, complete := reconcileRepairJournal(store, target, &journal)
			contents, readErr := os.ReadFile(targetPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if applied != test.wantApplied || complete != test.wantComplete || journal.State != test.wantState || string(contents) != test.wantFinalContents || journal.Operations[0].Restored != test.wantRestored {
				t.Fatalf("applied=%t complete=%t state=%s contents=%q restored=%t", applied, complete, journal.State, contents, journal.Operations[0].Restored)
			}
		})
	}
}

func repairPacketFixture(t *testing.T) RepairIssuePacket {
	t.Helper()
	now := time.Unix(100, 0).UTC()
	project, sprintSlug := "alpha", "38-repair"
	attemptID, err := NewQASemanticAttemptID(project, sprintSlug, QASemanticIdentity{GovernedInputFingerprint: strings.Repeat("1", 64), ImplementationFingerprint: strings.Repeat("2", 64), ReviewFingerprint: strings.Repeat("3", 64), PolicyFingerprint: strings.Repeat("4", 64)})
	if err != nil {
		t.Fatal(err)
	}
	mapID, err := NewQAMapID(project, sprintSlug, attemptID, QASemanticIdentity{GovernedInputFingerprint: strings.Repeat("1", 64), ImplementationFingerprint: strings.Repeat("2", 64), ReviewFingerprint: strings.Repeat("3", 64), PolicyFingerprint: strings.Repeat("4", 64)})
	if err != nil {
		t.Fatal(err)
	}
	groupID, _ := NewQAV2ID("group", project, sprintSlug, attemptID, "group")
	issueID, _ := NewQAV2ID("issue", project, sprintSlug, groupID, "issue")
	adjudicationID, _ := NewQAV2ID("adjudication", project, sprintSlug, attemptID, "adjudication")
	evidenceID, _ := NewQAV2ID("evidence", project, sprintSlug, attemptID, "evidence")
	planID, _ := NewQAV2ID("plan", project, sprintSlug, attemptID, "plan")
	shardID, _ := NewQAShardID(project, sprintSlug, mapID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"internal/a.go"}, ContextPaths: []string{"internal/a.go"}, ExpectationRefs: []string{"AC-1"}})
	runID, err := NewRepairRunID(project, sprintSlug, attemptID, issueID, RepairModeManual, now)
	if err != nil {
		t.Fatal(err)
	}
	checks := make([]RepairCheckDescriptor, 0, len(RepairGateOrder()))
	for _, gate := range RepairGateOrder() {
		id, idErr := NewRepairCheckID(runID, gate, string(gate))
		if idErr != nil {
			t.Fatal(idErr)
		}
		checks = append(checks, RepairCheckDescriptor{ID: id, Gate: gate, Executable: "go", Args: []string{"test", "./internal/sprint"}, Timeout: time.Minute, OutputLimit: 1024, Expected: "exit 0", SourcePlanID: planID})
	}
	return RepairIssuePacket{
		Project: project, Sprint: sprintSlug, QAAttemptID: attemptID, RepairRunID: runID,
		Issue:          QAIssue{ID: issueID, RootCauseGroupID: groupID, Title: "bug", IssueClass: "logic", Severity: "medium", Location: "internal/a.go", EvidenceIDs: []string{evidenceID}, RepairEligible: true},
		RootCauseGroup: QARootCauseGroup{ID: groupID, Claim: "bug", IssueClass: "logic", Location: "internal/a.go", EvidenceIDs: []string{evidenceID}},
		AdjudicationID: adjudicationID, EvidenceIDs: []string{evidenceID}, PlanIDs: []string{planID}, MapID: mapID,
		ShardIDs: []string{shardID}, ExpectationRefs: []string{"AC-1"}, ExactReproducer: checks[0], Checks: checks,
		AllowedPaths: []string{"internal/a.go"}, ForbiddenPaths: []string{"internal/a_test.go"}, AcceptanceCriteria: []string{"exact reproducer passes"},
		Mode: RepairModeManual, Budgets: DefaultRepairBudgets(), Target: QATargetIdentity{Fingerprint: strings.Repeat("a", 64), GitWorktree: strings.Repeat("b", 64)},
		GovernedInputFingerprint: strings.Repeat("1", 64), ImplementationFingerprint: strings.Repeat("2", 64), ReviewFingerprint: strings.Repeat("3", 64), SmokeFingerprint: strings.Repeat("4", 64), PolicyFingerprint: strings.Repeat("5", 64), IsolationFingerprint: strings.Repeat("6", 64), PreparedAt: now,
	}
}
