package sprint

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testQAFingerprint = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestQAPhaseAndTheoryOutcomeEnumsAreClosed(t *testing.T) {
	if got := QAPhaseStatuses(); len(got) != 11 || got[0] != QAPhaseMissing || got[len(got)-1] != QAPhaseInvalid {
		t.Fatalf("QAPhaseStatuses() = %v", got)
	}
	if got := QATheoryOutcomes(); len(got) != 7 || got[0] != QATheoryConfirmed || got[len(got)-1] != QATheoryNotApplicable {
		t.Fatalf("QATheoryOutcomes() = %v", got)
	}
	if containsQAPhase("unknown") || containsQATheoryOutcome("unknown") {
		t.Fatal("unknown enum value accepted")
	}
}

func TestQAIdentifiersAreStableAndScoped(t *testing.T) {
	identity := QASemanticIdentity{GovernedInputFingerprint: testQAFingerprint, ImplementationFingerprint: strings.Repeat("b", 64), ReviewFingerprint: strings.Repeat("c", 64), PolicyFingerprint: strings.Repeat("d", 64), ChangedPaths: []string{"a.go", "b.go"}}
	first, err := NewQASemanticAttemptID("alpha", "01-test", identity)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewQASemanticAttemptID("alpha", "01-test", identity)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !validQAID(first) || !strings.HasPrefix(first, "qa-v1-attempt-") {
		t.Fatalf("unstable or malformed IDs: %q %q", first, second)
	}
	identity.ChangedPaths = append(identity.ChangedPaths, "c.go")
	changed, _ := NewQASemanticAttemptID("alpha", "01-test", identity)
	if changed == first {
		t.Fatal("changed identity reused an ID")
	}
	if _, err := NewQASemanticAttemptID("../alpha", "01-test", identity); err == nil {
		t.Fatal("unsafe scope accepted")
	}
}

func TestQASettingsRejectEveryInvalidBudgetClass(t *testing.T) {
	valid := QASettings{Runtime: StageRuntime{Model: "openai/test", Variant: "high"}, Budgets: DefaultQABudgets(), Sources: []QAEffectiveSource{{Field: "qa.model", Source: "workspace"}}}
	if err := ValidateQASettings(valid); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*QASettings){
		"model":    func(s *QASettings) { s.Runtime.Model = "" },
		"zero":     func(s *QASettings) { s.Budgets.ChangedPaths = 0 },
		"negative": func(s *QASettings) { s.Budgets.RuntimeRetries = -1 },
		"maximum": func(s *QASettings) {
			s.Budgets.ConcurrentInvestigators = MaximumQABudgets().ConcurrentInvestigators + 1
		},
		"duration": func(s *QASettings) { s.Budgets.RunTimeout = 0 },
		"source":   func(s *QASettings) { s.Sources = []QAEffectiveSource{{Field: "qa.model"}} },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			candidate.Sources = append([]QAEffectiveSource(nil), valid.Sources...)
			mutate(&candidate)
			if err := ValidateQASettings(candidate); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestQATheoryRequiresCompleteFalsifiableRecordForEveryOutcome(t *testing.T) {
	attemptID, _ := NewQASemanticAttemptID("alpha", "01-test", QASemanticIdentity{ChangedPaths: []string{"a.go"}})
	shardID, _ := NewQAShardID("alpha", "01-test", attemptID, QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"a.go"}, BehavioralConcerns: []string{"behavior"}, ExpectationRefs: []string{"REQ-1"}})
	theoryID, _ := NewQATheoryID("alpha", "01-test", shardID, QATheoryIdentity{Claim: "a fails", Basis: "branch", VerificationSurface: "a.go", ExpectationRefs: []string{"REQ-1"}})
	base := QATheory{SchemaVersion: 1, ID: theoryID, ShardID: shardID, Claim: "a fails", Basis: "branch", VerificationSurface: "a.go", ExpectationRefs: []string{"REQ-1"}, SeverityIfConfirmed: "medium", ConfirmationCondition: "existing check fails", RefutationCondition: "existing check passes", InconclusiveCondition: "check unavailable", SafeEvidenceStrategy: "read source and run check", ImplementationFingerprint: testQAFingerprint, AttemptHistory: []QAInvestigatorAttempt{{ID: attemptID, Number: 1, StartedAt: time.Now()}}, OutcomeReason: "observed result"}
	for _, outcome := range QATheoryOutcomes() {
		candidate := base
		candidate.Outcome = outcome
		if err := ValidateQATheory(candidate); err != nil {
			t.Fatalf("outcome %q: %v", outcome, err)
		}
	}
	candidate := base
	candidate.Outcome = QATheoryConfirmed
	candidate.RefutationCondition = ""
	if err := ValidateQATheory(candidate); err == nil {
		t.Fatal("incomplete theory accepted")
	}
	candidate = base
	candidate.Outcome = "maybe"
	if err := ValidateQATheory(candidate); err == nil {
		t.Fatal("unknown outcome accepted")
	}
	candidate = base
	candidate.Outcome = QATheoryConfirmed
	candidate.ImplementationFingerprint = "short"
	if err := ValidateQATheory(candidate); err == nil {
		t.Fatal("invalid fingerprint accepted")
	}
}

func TestQAErrorPreservesCauseAndRecovery(t *testing.T) {
	cause := errors.New("disk full")
	err := NewQAError(QAErrorPersistenceFailure, "publish", "state write failed", cause)
	if !errors.Is(err, cause) || err.Recovery == "" || err.Category != QAErrorPersistenceFailure {
		t.Fatalf("QA error = %+v", err)
	}
	classified, ok := AsQAError(err)
	if !ok || classified != err {
		t.Fatalf("AsQAError = %+v, %v", classified, ok)
	}
}

func TestQARepairStorePublishesPrivateDigestBoundRecords(t *testing.T) {
	root := t.TempDir()
	sp := Sprint{Project: "alpha", Slug: "38-repair", Path: filepath.Join(root, "projects", "alpha", "sprints", "38-repair")}
	if err := os.MkdirAll(sp.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	packet, err := FinalizeRepairPacket(repairPacketFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(200, 0).UTC()
	token := QAWriterToken{RunID: "operation-1", OperationalAttemptID: "attempt-1", FencingGeneration: 1}
	fence := func(got QAWriterToken) error {
		if got != token {
			return errors.New("stale")
		}
		return nil
	}
	store := NewQAStore(root, sp).WithWriterFence(fence)
	flow := NewFlowState(sp, emptyPlanningStageStates(sp), now)
	state := RepairState{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, Mode: RepairModeManual, Phase: RepairPhasePrepared, Freshness: RepairFreshness{Current: true}, Run: QARunCorrelation{Lifecycle: QARunAccepted, RunID: token.RunID, OperationalAttemptID: token.OperationalAttemptID, FencingGeneration: token.FencingGeneration}, Deadline: now.Add(time.Hour), NextAction: "Review and confirm the packet.", UpdatedAt: now}
	if err := store.PublishRepairPacket(packet, state, flow, token); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadRepairState()
	if err != nil || loaded.Packet == nil || loaded.Packet.Digest == "" {
		t.Fatalf("loaded state = %+v, err=%v", loaded, err)
	}
	loadedPacket, err := store.LoadRepairPacket(packet.QAAttemptID, packet.RepairRunID)
	if err != nil || loadedPacket.PacketDigest != packet.PacketDigest {
		t.Fatalf("loaded packet digest = %q, err=%v", loadedPacket.PacketDigest, err)
	}
	for _, rel := range []string{QARepairStateRelPath(sp), QARepairPacketRelPath(sp, packet.QAAttemptID, packet.RepairRunID)} {
		info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode for %s = %v, err=%v", rel, info.Mode().Perm(), statErr)
		}
	}

	confirmation, err := FinalizeRepairConfirmation(RepairConfirmation{Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, PacketDigest: packet.PacketDigest, Target: packet.Target, Mode: RepairModeManual, Budgets: packet.Budgets, GovernedInputFingerprint: packet.GovernedInputFingerprint, PolicyFingerprint: packet.PolicyFingerprint, OperationRunID: token.RunID, OperationalAttemptID: token.OperationalAttemptID, FencingGeneration: token.FencingGeneration, Confirmer: "operator", ConfirmedAt: now}, packet)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PublishRepairConfirmation(confirmation, loaded, flow, token); err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.LoadRepairState()
	if err != nil || confirmed.Phase != RepairPhaseConfirmed || confirmed.Confirmation == nil {
		t.Fatalf("confirmed state = %+v, err=%v", confirmed, err)
	}
	result := RepairResult{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, Mode: RepairModeManual, Outcome: RepairOutcomeVerified, Reason: "all frozen gates passed", StopReason: RepairStopVerified, Consumed: RepairConsumed{MutationCycles: 1}, Target: packet.Target, CleanupComplete: true, ProductionApplied: true, CompleteLadder: true, Evidence: []QAArtifactRef{*confirmed.Packet, *confirmed.Confirmation}, NextAction: "Review retained evidence.", CompletedAt: now.Add(time.Minute)}
	if err := store.PublishRepairResult(result, confirmed, flow, token); err != nil {
		t.Fatal(err)
	}
	terminal, err := store.LoadRepairState()
	if err != nil || terminal.Outcome != RepairOutcomeVerified || terminal.Result == nil {
		t.Fatalf("terminal state = %+v, err=%v", terminal, err)
	}
	proof := ManualRepairProof{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, RepairRunID: packet.RepairRunID, PacketDigest: packet.PacketDigest, ResultDigest: terminal.Result.Digest, Outcome: result.Outcome, Target: packet.Target, ProtocolFingerprint: testQAFingerprint, ImplementationFingerprint: result.Target.Fingerprint, PolicyFingerprint: packet.PolicyFingerprint, IsolationFingerprint: packet.IsolationFingerprint, GovernedInputFingerprint: packet.GovernedInputFingerprint, RuntimeFingerprint: strings.Repeat("b", 64), CleanupComplete: true, ProductionApplied: true, CompleteLadder: true, PublishedAt: now.Add(2 * time.Minute)}
	if err := store.PublishManualRepairProof(proof, packet, result, testQAFingerprint, strings.Repeat("b", 64), token); err != nil {
		t.Fatal(err)
	}
	if loadedProof, err := store.LoadManualRepairProof(); err != nil || loadedProof.RepairRunID != packet.RepairRunID {
		t.Fatalf("proof = %+v, err=%v", loadedProof, err)
	}
}

func TestQARepairStoreRechecksWriterBeforeRename(t *testing.T) {
	root := t.TempDir()
	sp := Sprint{Project: "alpha", Slug: "38-repair", Path: filepath.Join(root, "projects", "alpha", "sprints", "38-repair")}
	if err := os.MkdirAll(sp.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	packet, err := FinalizeRepairPacket(repairPacketFixture(t))
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error {
		calls++
		if calls > 1 {
			return errors.New("writer moved")
		}
		return nil
	})
	now := time.Now().UTC()
	token := QAWriterToken{RunID: "operation-1", OperationalAttemptID: "attempt-1", FencingGeneration: 1}
	state := RepairState{SchemaVersion: QARepairSchemaVersion, Project: sp.Project, Sprint: sp.Slug, QAAttemptID: packet.QAAttemptID, RepairRunID: packet.RepairRunID, Mode: RepairModeManual, Phase: RepairPhasePrepared, Freshness: RepairFreshness{Current: true}, Run: QARunCorrelation{Lifecycle: QARunAccepted, RunID: token.RunID, OperationalAttemptID: token.OperationalAttemptID, FencingGeneration: token.FencingGeneration}, Deadline: now.Add(time.Hour), NextAction: "Review.", UpdatedAt: now}
	if err := store.PublishRepairPacket(packet, state, NewFlowState(sp, emptyPlanningStageStates(sp), now), token); err == nil {
		t.Fatal("stale writer published packet")
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(QARepairPacketRelPath(sp, packet.QAAttemptID, packet.RepairRunID)))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("packet survived stale-writer rejection: %v", err)
	}
}

func TestQAStorePublishesPrivateRecordsPointerLastAndLoadsStrictly(t *testing.T) {
	root, sp, publication := qaPublicationFixture(t)
	token := QAWriterToken{RunID: "run-1", OperationalAttemptID: "op-1", FencingGeneration: 3}
	store := NewQAStore(root, sp).WithWriterFence(func(got QAWriterToken) error {
		if got != token {
			return errors.New("stale")
		}
		return nil
	})
	if err := store.Publish(publication, token); err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if state.Map == nil || state.Map.Digest == "" || state.CurrentAttemptID != publication.Map.SemanticAttemptID {
		t.Fatalf("published state = %+v", state)
	}
	if _, err := store.LoadMap(state.CurrentAttemptID); err != nil {
		t.Fatal(err)
	}
	loadedFlow, err := LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	if loadedFlow.QA == nil || loadedFlow.QA.StateDigest == "" || loadedFlow.QA.Phase != QAPhaseMapped {
		t.Fatalf("flow QA summary = %+v", loadedFlow.QA)
	}
	for _, rel := range []string{QAVerificationStateRelPath(sp), QAMapRelPath(sp, state.CurrentAttemptID), QAShardRelPath(sp, state.CurrentAttemptID, publication.Shards[0].ID)} {
		path, err := resolveSprintContained(root, sp, rel)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode = %o", rel, info.Mode().Perm())
		}
	}
	verification := filepath.Join(sp.Path, "verification")
	if info, err := os.Stat(verification); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("verification mode = %v, err=%v", info.Mode().Perm(), err)
	}
}

func TestQAStoreRejectsUnknownFieldsVersionsTrailingJSONAndModes(t *testing.T) {
	root, sp, _ := qaPublicationFixture(t)
	store := NewQAStore(root, sp)
	path, err := store.StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"unknown version": `{"schema_version":3}`,
		"unknown field":   `{"schema_version":1,"unknown":true}`,
		"trailing":        `{"schema_version":1} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := store.LoadState()
			if err == nil {
				t.Fatal("expected strict decoding error")
			}
			qaErr, ok := AsQAError(err)
			if !ok {
				t.Fatalf("error = %T %v", err, err)
			}
			if name == "unknown version" && qaErr.Category != QAErrorUnknownSchema {
				t.Fatalf("category = %q", qaErr.Category)
			}
		})
	}
	if err := os.WriteFile(path, []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadState(); err == nil {
		t.Fatal("public mode accepted")
	}
}

func TestQAStoreRejectsSymlinkEscapeAndStaleWriter(t *testing.T) {
	root, sp, publication := qaPublicationFixture(t)
	external := t.TempDir()
	if err := os.Symlink(external, filepath.Join(sp.Path, "verification")); err != nil {
		t.Fatal(err)
	}
	if _, err := NewQAStore(root, sp).StatePath(); err == nil {
		t.Fatal("symlink verification root accepted")
	}
	if err := os.Remove(filepath.Join(sp.Path, "verification")); err != nil {
		t.Fatal(err)
	}
	token := QAWriterToken{RunID: "run-1", OperationalAttemptID: "op-1", FencingGeneration: 1}
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return context.Canceled })
	if err := store.Publish(publication, token); err == nil {
		t.Fatal("stale writer published")
	}
	if _, err := os.Stat(filepath.Join(sp.Path, "verification", "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale writer created state: %v", err)
	}
}

func TestQAAtomicFailurePreservesPriorStateAndMapIsImmutable(t *testing.T) {
	root, sp, publication := qaPublicationFixture(t)
	token := QAWriterToken{RunID: "run-1", OperationalAttemptID: "op-1", FencingGeneration: 1}
	fence := func(QAWriterToken) error { return nil }
	store := NewQAStore(root, sp).WithWriterFence(fence)
	if err := store.Publish(publication, token); err != nil {
		t.Fatal(err)
	}
	prior, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	changed := publication
	changed.State.NextAction = "changed next action"
	changed.Map = nil
	failing := store.WithHooks(QAStateHooks{BeforeRename: func(kind, path string) error {
		if kind == "state" {
			return errors.New("injected rename failure")
		}
		return nil
	}})
	if err := failing.Publish(changed, token); err == nil {
		t.Fatal("expected injected failure")
	}
	after, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if after.NextAction != prior.NextAction {
		t.Fatalf("state changed after failed rename: %+v", after)
	}

	mapChanged := publication
	mapCopy := *publication.Map
	mapCopy.Shards = append([]QAShard(nil), publication.Map.Shards...)
	mapCopy.Shards[0].Title = "different bytes"
	mapChanged.Map = &mapCopy
	if err := store.Publish(mapChanged, token); err == nil {
		t.Fatal("immutable map replacement accepted")
	}
}

func TestQAEvidencePublicationLoadsAndRollsBackCanonicalFiles(t *testing.T) {
	root, sp, initial := qaPublicationFixture(t)
	token := QAWriterToken{RunID: "run-1", OperationalAttemptID: "op-1", FencingGeneration: 1}
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	if err := store.Publish(initial, token); err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	flow, err := LoadFlowState(root, sp)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := FreezeQAEvidencePlan(sp.Project, sp.Slug, QAEvidencePlan{AttemptID: state.CurrentAttemptID, ShardID: initial.Shards[0].ID, ExpectationRefs: []string{"REQ-1"}, Kind: QACheckFact, ConfirmationCondition: "fact observed", RefutationCondition: "fact absent", InconclusiveCondition: "fact unavailable", ApprovedPaths: []string{"internal/a.go"}, Executable: "true", Timeout: time.Second, OutputLimit: 1024, CleanupRequired: true, GovernedInputFingerprint: initial.Map.GovernedInputFingerprint, ImplementationFingerprint: initial.Map.ImplementationFingerprint, MapFingerprint: initial.Map.CheckCatalogFingerprint}, initial.Map.Budgets, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	evidenceID, _ := NewQAV2ID("evidence", sp.Project, sp.Slug, plan.ID, "pass")
	patchID, _ := NewQAV2ID("patch", sp.Project, sp.Slug, plan.ID, "candidate")
	patchContent := []byte("--- a/internal/a.go\n+++ b/internal/a.go\n@@ -1 +1 @@\n-old\n+new\n")
	patchRef := QAArtifactRef{Path: QAPatchRelPath(sp, state.CurrentAttemptID, patchID), Digest: hashBytes(normalizePatch(patchContent))}
	evidence := QAEvidenceRecord{SchemaVersion: QAEvidenceSchemaVersion, ID: evidenceID, PlanID: plan.ID, AttemptID: state.CurrentAttemptID, ShardID: plan.ShardID, WorkspaceID: "opaque", WorkspaceIdentity: strings.Repeat("e", 64), TargetIdentityBefore: initial.Map.ImplementationFingerprint, TargetIdentityAfter: initial.Map.ImplementationFingerprint, GovernedInputFingerprint: plan.GovernedInputFingerprint, ImplementationFingerprint: plan.ImplementationFingerprint, MapFingerprint: plan.MapFingerprint, Patch: &patchRef, Outcome: QAEvidencePass, ReasonCode: "fact_observed", Repeatable: true, Contained: true, Cleanup: QACleanupFacts{Attempted: true, DescendantsTerminated: true, WorkspaceRemoved: true, Complete: true}, CompletedAt: time.Unix(3, 0)}
	adjudication, err := AdjudicateQA(QAAdjudicationRequest{Project: sp.Project, Sprint: sp.Slug, AttemptID: state.CurrentAttemptID, MapFingerprint: plan.MapFingerprint, Plans: []QAEvidencePlan{plan}, Evidence: []QAEvidenceRecord{evidence}, Budgets: initial.Map.Budgets, Now: time.Unix(4, 0)})
	if err != nil {
		t.Fatal(err)
	}
	assessmentID, _ := NewQAV2ID("assessment", sp.Project, sp.Slug, state.CurrentAttemptID, "pass")
	assessment := QAAssessmentRecord{SchemaVersion: QAEvidenceSchemaVersion, ID: assessmentID, AttemptID: state.CurrentAttemptID, ReviewVerdict: ReviewPass, ReviewFingerprint: initial.Map.ReviewFingerprint, Assessment: AssessmentPass, EvidenceTotal: 1, NextAction: "Proceed to independent review.", CompletedAt: time.Unix(5, 0)}
	report, err := RenderQAReport(sp.Project, sp.Slug, initial.Map.GovernedInputFingerprint, []QAEvidenceRecord{evidence}, adjudication, assessment)
	if err != nil {
		t.Fatal(err)
	}
	state.Phase, state.CompletedShards, state.Run.Lifecycle, state.Run.TerminalResult = QAPhaseCompleted, 1, QARunTerminal, QATerminalCompleted
	state.NextAction = assessment.NextAction
	publication := QAPublication{State: state, Flow: flow, Evidence: &QAEvidencePublication{Plans: []QAEvidencePlan{plan}, Records: []QAEvidenceRecord{evidence}, Patches: []QAPatchRecord{{ID: patchID, Content: patchContent}}, Adjudication: &adjudication, Assessment: &assessment, Report: []byte(report), Budgets: initial.Map.Budgets}}
	if err := store.Publish(publication, token); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.EvidenceCount != 1 || loaded.Adjudication == nil || loaded.Assessment == nil || loaded.CanonicalReport == nil {
		t.Fatalf("evidence state = %+v", loaded)
	}
	if _, err := store.LoadEvidence(state.CurrentAttemptID, evidence.ID); err != nil {
		t.Fatal(err)
	}
	if retained, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(patchRef.Path))); err != nil || !bytes.Equal(retained, normalizePatch(patchContent)) {
		t.Fatalf("retained patch=%q err=%v", retained, err)
	}
	priorReport, err := os.ReadFile(filepath.Join(sp.Path, "qa.md"))
	if err != nil {
		t.Fatal(err)
	}
	failing := store.WithHooks(QAStateHooks{BeforeRename: func(kind, _ string) error {
		if kind == "state" {
			return errors.New("injected state failure")
		}
		return nil
	}})
	publication.State = loaded
	publication.State.NextAction = "must not persist"
	publication.Evidence.Report = []byte("# QA\n\nreplacement\n")
	if err := failing.Publish(publication, token); err == nil {
		t.Fatal("expected canonical publication failure")
	}
	after, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	afterReport, err := os.ReadFile(filepath.Join(sp.Path, "qa.md"))
	if err != nil {
		t.Fatal(err)
	}
	if after.NextAction != loaded.NextAction || !bytes.Equal(afterReport, priorReport) {
		t.Fatal("failed publication did not preserve the prior canonical state and report")
	}
}

func qaPublicationFixture(t *testing.T) (string, Sprint, QAPublication) {
	t.Helper()
	root := t.TempDir()
	sp := Sprint{Project: "alpha", Slug: "01-test", Path: filepath.Join(root, "projects", "alpha", "sprints", "01-test")}
	if err := os.MkdirAll(sp.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	semantic := QASemanticIdentity{GovernedInputFingerprint: testQAFingerprint, ImplementationFingerprint: strings.Repeat("b", 64), ReviewFingerprint: strings.Repeat("c", 64), PolicyFingerprint: strings.Repeat("d", 64), ChangedPaths: []string{"internal/a.go"}}
	attemptID, err := NewQASemanticAttemptID(sp.Project, sp.Slug, semantic)
	if err != nil {
		t.Fatal(err)
	}
	mapID, err := NewQAMapID(sp.Project, sp.Slug, attemptID, semantic)
	if err != nil {
		t.Fatal(err)
	}
	shardIdentity := QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: []string{"internal/a.go"}, BehavioralConcerns: []string{"a behavior"}, ExpectationRefs: []string{"REQ-1"}}
	shardID, err := NewQAShardID(sp.Project, sp.Slug, mapID, shardIdentity)
	if err != nil {
		t.Fatal(err)
	}
	shard := QAShard{SchemaVersion: 1, ID: shardID, AttemptID: attemptID, Kind: QAShardPrimary, Title: "a behavior", ChangedPaths: []string{"internal/a.go"}, BehavioralConcerns: []string{"a behavior"}, ExpectationRefs: []string{"REQ-1"}, Phase: QAPhaseMapped}
	qaMap := QAMap{SchemaVersion: 1, ID: mapID, Project: sp.Project, Sprint: sp.Slug, SemanticAttemptID: attemptID, GovernedInputFingerprint: semantic.GovernedInputFingerprint, ImplementationFingerprint: semantic.ImplementationFingerprint, ReviewFingerprint: semantic.ReviewFingerprint, PolicyFingerprint: semantic.PolicyFingerprint, CheckCatalogFingerprint: strings.Repeat("e", 64), Budgets: DefaultQABudgets(), Target: QATargetIdentity{Fingerprint: semantic.ImplementationFingerprint}, Coverage: QACoverage{ChangedPaths: []string{"internal/a.go"}, PrimaryOwners: map[string]string{"internal/a.go": shardID}}, Shards: []QAShard{shard}}
	now := time.Now().UTC()
	state := QAState{SchemaVersion: 1, Project: sp.Project, Sprint: sp.Slug, Phase: QAPhaseMapped, Freshness: QAFreshness{Current: true, GovernedInputFingerprint: semantic.GovernedInputFingerprint, ImplementationFingerprint: semantic.ImplementationFingerprint, ReviewFingerprint: semantic.ReviewFingerprint, PolicyFingerprint: semantic.PolicyFingerprint}, CompletedShards: 0, TotalShards: 1, Cancellation: QACancellation{}, Run: QARunCorrelation{Lifecycle: QARunAccepted, RunID: "run-1", OperationalAttemptID: "op-1", FencingGeneration: 1}, NextAction: "Run QA.", UpdatedAt: now}
	flow := NewFlowState(sp, emptyPlanningStageStates(sp), now)
	return root, sp, QAPublication{Map: &qaMap, Shards: []QAShard{shard}, State: state, Flow: flow}
}
