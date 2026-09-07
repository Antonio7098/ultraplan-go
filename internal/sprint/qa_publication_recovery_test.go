package sprint

import (
	"strings"
	"testing"
	"time"
)

func TestQAInterruptedSynthesisRequiresRetainedCheckpoints(t *testing.T) {
	root, sp, publication := qaPublicationFixture(t)
	qaMap, investigated := qaSynthesisFixture(t)
	shards := append([]QAShard(nil), qaMap.Shards...)
	for i := range shards {
		if shards[i].ID == investigated[0].ID {
			shards[i].Theories = investigated[0].Theories[:1]
		}
	}
	theoryID := investigated[0].Theories[0].ID
	group := QAArbiterGroup{ID: "qa-v1-arbiter-group-" + strings.Repeat("a", 24), TheoryIDs: []string{theoryID}, Round: 1, SessionID: "retained", Provider: "openai", Model: "qa", RuntimeStoreRef: "/retained/store", WorkspaceID: strings.Repeat("a", 64)}
	synthesis, err := SynthesizeQA(qaMap, shards)
	if err != nil {
		t.Fatal(err)
	}
	synthesis.Arbitration = &QAArbitration{SchemaVersion: QASchemaVersion, MapID: qaMap.ID, Groups: []QAArbiterGroup{group}}
	adjudication, err := AdjudicateQA(QAAdjudicationRequest{Project: sp.Project, Sprint: sp.Slug, AttemptID: qaMap.SemanticAttemptID, MapFingerprint: qaMap.CheckCatalogFingerprint, Budgets: qaMap.Budgets, Now: time.Unix(1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	publication.Map, publication.Shards, publication.Synthesis = &qaMap, shards, &synthesis
	publication.State.CurrentAttemptID = qaMap.SemanticAttemptID
	publication.State.Phase = QAPhaseSynthesizing
	publication.State.Freshness = QAFreshness{Current: true, GovernedInputFingerprint: qaMap.GovernedInputFingerprint, ImplementationFingerprint: qaMap.ImplementationFingerprint, ReviewFingerprint: qaMap.ReviewFingerprint, PolicyFingerprint: qaMap.PolicyFingerprint}
	publication.Evidence = &QAEvidencePublication{Budgets: qaMap.Budgets, Adjudication: &adjudication}
	token := QAWriterToken{RunID: "run-1", OperationalAttemptID: "op-1", FencingGeneration: 1}
	store := NewQAStore(root, sp).WithWriterFence(func(QAWriterToken) error { return nil })
	if err := store.Publish(publication, token); err != nil {
		t.Fatal(err)
	}
	state, err := store.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	group.Round++
	if err := store.PublishArbiterSessionGroups(qaMap.SemanticAttemptID, []QAArbiterGroup{group}, token); err != nil {
		t.Fatal(err)
	}
	synthesis.Arbitration.Groups = []QAArbiterGroup{group}
	path, _ := store.synthesisPath(qaMap.SemanticAttemptID)
	for _, test := range []string{"unknown_checkpoint", "changed_synthesis", "wrong_map", "completed_state", "valid"} {
		t.Run(test, func(t *testing.T) {
			candidate, current := synthesis, state
			arbitration := *synthesis.Arbitration
			candidate.Arbitration = &arbitration
			switch test {
			case "unknown_checkpoint":
				other := group
				other.Round++
				arbitration.Groups = []QAArbiterGroup{other}
			case "changed_synthesis":
				candidate.NextAction = "unsubstantiated replacement"
			case "wrong_map":
				candidate.MapID = "qa-v1-map-" + strings.Repeat("f", 24)
			case "completed_state":
				current.Phase = QAPhaseCompleted
			}
			if _, err := store.writeRecord("synthesis", path, &candidate, false); err != nil {
				t.Fatal(err)
			}
			recovered, err := store.recoverInterruptedSynthesis(&current)
			if err != nil || recovered != (test == "valid") {
				t.Fatalf("recovered=%t err=%v", recovered, err)
			}
			if recovered {
				if current.Phase != QAPhaseSynthesizing || current.Synthesis.Digest == state.Synthesis.Digest {
					t.Fatal("recovery either failed to update progress or claimed completion")
				}
				if err := store.verifyReference(*current.Synthesis); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
