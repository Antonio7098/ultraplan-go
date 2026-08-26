package sprint

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestQASynthesisPreservesOutcomesContradictionsAndBoundedFollowUp(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	result, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	if result.OutcomeCounts[QATheoryConfirmed] != 1 || result.OutcomeCounts[QATheoryRefuted] != 1 || result.OutcomeCounts[QATheoryCrossShard] != 1 || result.OutcomeCounts[QATheoryInconclusive] != 1 {
		t.Fatalf("outcome counts = %+v", result.OutcomeCounts)
	}
	if len(result.Contradictions) != 1 || len(result.Contradictions[0]) != 2 {
		t.Fatalf("contradictions = %+v", result.Contradictions)
	}
	if len(result.FollowUpShards) != 2 {
		t.Fatalf("follow-up shards = %+v", result.FollowUpShards)
	}
	for _, shard := range result.FollowUpShards {
		if shard.Kind != QAShardFollowUp || len(shard.ParentTheoryIDs) != 1 {
			t.Fatalf("follow-up = %+v", shard)
		}
	}
	data, err := NormalizedQASynthesisBytes(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"issue", "repair_eligible", "review_verdict", "qa.md"} {
		if bytes.Contains(bytes.ToLower(data), []byte(forbidden)) {
			t.Fatalf("synthesis contains forbidden field %q: %s", forbidden, data)
		}
	}
}

func TestQASynthesisIsDeterministicAndCapsFollowUp(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	qaMap.Budgets.FollowUpShards = 1
	first, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(shards)-1; i < j; i, j = i+1, j-1 {
		shards[i], shards[j] = shards[j], shards[i]
	}
	second, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := NormalizedQASynthesisBytes(first)
	secondBytes, _ := NormalizedQASynthesisBytes(second)
	if len(first.FollowUpShards) != 1 || !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("nondeterministic synthesis:\n%s\n%s", firstBytes, secondBytes)
	}
}

func TestQASynthesisChallengerRecordsAreValidatedAndFingerprintPureOutput(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	identity := QAChallengeIdentity{TheoryIDs: []string{shards[0].Theories[0].ID}, Claim: "The retained evidence may not cover the boundary.", Basis: "The theory is scoped to one producer.", SafeEvidenceStrategy: "Compare the retained consumer evidence.", EvidenceRefs: []string{"consumer-boundary"}}
	challenge, err := BuildQAChallenge(qaMap.Project, qaMap.Sprint, qaMap, identity)
	if err != nil {
		t.Fatal(err)
	}
	first, err := SynthesizeQAWithChallenges(qaMap, shards, []QAChallenge{challenge})
	if err != nil {
		t.Fatal(err)
	}
	second, err := SynthesizeQAWithChallenges(qaMap, shards, []QAChallenge{challenge})
	if err != nil {
		t.Fatal(err)
	}
	firstBytes, _ := NormalizedQASynthesisBytes(first)
	secondBytes, _ := NormalizedQASynthesisBytes(second)
	if len(first.Challenges) != 1 || !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("challenged synthesis is not stable:\n%s\n%s", firstBytes, secondBytes)
	}
	challenge.TheoryIDs = []string{"qa-v1-theory-aaaaaaaaaaaaaaaaaaaaaaaa"}
	if _, err := SynthesizeQAWithChallenges(qaMap, shards, []QAChallenge{challenge}); err == nil {
		t.Fatal("accepted challenger record for an unknown theory")
	}
}

func TestQASynthesisHydrationRequiresRetainedTerminalFollowUpShard(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	synthesis, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	if len(synthesis.FollowUpShards) == 0 {
		t.Fatal("fixture did not propose a follow-up")
	}
	retained := append([]QAShard(nil), shards...)
	for _, follow := range synthesis.FollowUpShards {
		follow.Phase = QAPhaseCompleted
		retained = append(retained, follow)
	}
	if err := hydrateQASynthesisFollowUps(&synthesis, retained); err != nil {
		t.Fatal(err)
	}
	if synthesis.FollowUpShards[0].Phase != QAPhaseCompleted {
		t.Fatalf("hydrated follow-up = %+v", synthesis.FollowUpShards[0])
	}
	missing, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	if err := hydrateQASynthesisFollowUps(&missing, shards); err == nil {
		t.Fatal("unretained follow-up was accepted")
	}
}

func TestQASynthesisFollowUpsUseOneCumulativeAttemptBudget(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	qaMap.Budgets.FollowUpShards = 3
	first, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	initial := pendingQASynthesisFollowUps(first, shards, qaMap.Budgets.FollowUpShards)
	if len(initial) != 2 {
		t.Fatalf("initial pending follow-ups = %d, want 2", len(initial))
	}
	for i := range initial {
		initial[i].Phase = QAPhaseCompleted
	}
	parent := initial[0]
	identity := QATheoryIdentity{Claim: "new follow-up finding", Basis: "follow-up evidence", VerificationSurface: "linked behavior", ExpectationRefs: []string{"REQ-1"}}
	theoryID, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, parent.ID, identity)
	if err != nil {
		t.Fatal(err)
	}
	parent.Theories = []QATheory{{SchemaVersion: QASchemaVersion, ID: theoryID, ShardID: parent.ID, Claim: identity.Claim, Basis: identity.Basis, VerificationSurface: identity.VerificationSurface, ExpectationRefs: identity.ExpectationRefs, SeverityIfConfirmed: "medium", ConfirmationCondition: "evidence confirms", RefutationCondition: "evidence refutes", InconclusiveCondition: "evidence unavailable", SafeEvidenceStrategy: "read linked paths", ImplementationFingerprint: qaMap.ImplementationFingerprint, AttemptHistory: []QAInvestigatorAttempt{{ID: qaMap.SemanticAttemptID, Number: 1, StartedAt: time.Now().UTC()}}, Outcome: QATheoryInconclusive, OutcomeReason: "more evidence is required"}}
	initial[0] = parent
	retained := append(append([]QAShard(nil), shards...), initial...)
	second, err := SynthesizeQA(qaMap, retained)
	if err != nil {
		t.Fatal(err)
	}
	pending := pendingQASynthesisFollowUps(second, retained, qaMap.Budgets.FollowUpShards)
	if len(pending) != 1 {
		t.Fatalf("chained pending follow-ups = %d, want remaining budget of 1", len(pending))
	}
	pending[0].Phase = QAPhaseCompleted
	retained = append(retained, pending[0])
	if more := pendingQASynthesisFollowUps(second, retained, qaMap.Budgets.FollowUpShards); len(more) != 0 {
		t.Fatalf("pending follow-ups exceeded cumulative budget: %+v", more)
	}
}

func TestFinalizeQASynthesisUsesRetainedTerminalFollowUps(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	synthesis, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	retained := append([]QAShard(nil), shards...)
	for _, follow := range synthesis.FollowUpShards {
		follow.Phase = QAPhaseCompleted
		retained = append(retained, follow)
	}
	if err := finalizeQASynthesisFollowUps(&synthesis, qaMap, retained); err != nil {
		t.Fatal(err)
	}
	if len(synthesis.FollowUpShards) != 2 || synthesis.NextAction != "Inspect the retained theory outcomes." {
		t.Fatalf("final synthesis = %+v", synthesis)
	}
	for _, follow := range synthesis.FollowUpShards {
		if follow.Phase != QAPhaseCompleted {
			t.Fatalf("non-terminal final follow-up = %+v", follow)
		}
	}
	firstBytes, err := NormalizedQASynthesisBytes(synthesis)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(retained)-1; i < j; i, j = i+1, j-1 {
		retained[i], retained[j] = retained[j], retained[i]
	}
	second, err := SynthesizeQA(qaMap, retained)
	if err != nil {
		t.Fatal(err)
	}
	if err := finalizeQASynthesisFollowUps(&second, qaMap, retained); err != nil {
		t.Fatal(err)
	}
	secondBytes, err := NormalizedQASynthesisBytes(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstBytes, secondBytes) {
		t.Fatalf("final synthesis is not deterministic:\n%s\n%s", firstBytes, secondBytes)
	}
}

func qaSynthesisFixture(t *testing.T) (QAMap, []QAShard) {
	t.Helper()
	input := qaMapInputFixture()
	qaMap, err := BuildQAMap(input)
	if err != nil {
		t.Fatal(err)
	}
	var primary []QAShard
	for _, shard := range qaMap.Shards {
		if shard.Kind == QAShardPrimary {
			primary = append(primary, shard)
		}
	}
	if len(primary) < 2 {
		t.Fatal("fixture needs two primary shards")
	}
	now := time.Now().UTC()
	makeTheory := func(shard QAShard, claim string, outcome QATheoryOutcome) QATheory {
		identity := QATheoryIdentity{Claim: claim, Basis: "source branch", VerificationSurface: "public behavior", ExpectationRefs: []string{"REQ-1"}}
		id, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, identity)
		if err != nil {
			t.Fatal(err)
		}
		attempt := QAInvestigatorAttempt{ID: qaMap.SemanticAttemptID, Number: 1, StartedAt: now}
		return QATheory{SchemaVersion: 1, ID: id, ShardID: shard.ID, Claim: claim, Basis: identity.Basis, VerificationSurface: identity.VerificationSurface, ExpectationRefs: identity.ExpectationRefs, SeverityIfConfirmed: "medium", ConfirmationCondition: "existing evidence confirms", RefutationCondition: "existing evidence refutes", InconclusiveCondition: "evidence unavailable", SafeEvidenceStrategy: "read assigned paths", ImplementationFingerprint: qaMap.ImplementationFingerprint, AttemptHistory: []QAInvestigatorAttempt{attempt}, Outcome: outcome, OutcomeReason: "recorded result"}
	}
	primary[0].Theories = []QATheory{makeTheory(primary[0], "same claim", QATheoryConfirmed), makeTheory(primary[0], "needs context", QATheoryCrossShard)}
	primary[1].Theories = []QATheory{makeTheory(primary[1], "same claim", QATheoryRefuted), makeTheory(primary[1], "unknown edge", QATheoryInconclusive)}
	primary[0].RiskTags, primary[1].RiskTags = []string{"public-api"}, []string{"public-api"}
	return qaMap, primary
}

func TestQASynthesisRejectsStaleShard(t *testing.T) {
	qaMap, shards := qaSynthesisFixture(t)
	shards[0].AttemptID = "qa-v1-attempt-" + strings.Repeat("f", 24)
	if _, err := SynthesizeQA(qaMap, shards); err == nil {
		t.Fatal("stale shard accepted")
	}
}
