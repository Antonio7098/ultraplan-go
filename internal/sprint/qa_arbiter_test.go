package sprint

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestGroupQATheoriesUsesSemanticContextAffinityAndHonorsLimit(t *testing.T) {
	qaMap := QAMap{ID: "map", Foundation: &QAFoundation{Blocks: []QAContextBlock{{ID: "block-a"}, {ID: "block-b"}}}}
	shards := []QAShard{
		{ID: "shard-a", ChangedPaths: []string{"internal/a.go"}, ContextBlockIDs: []string{"block-a"}, Theories: []QATheory{{ID: "theory-a1", ShardID: "shard-a"}, {ID: "theory-a2", ShardID: "shard-a"}}},
		{ID: "shard-b", ChangedPaths: []string{"internal/b.go"}, ContextBlockIDs: []string{"block-b"}, Theories: []QATheory{{ID: "theory-b1", ShardID: "shard-b"}, {ID: "theory-b2", ShardID: "shard-b"}}},
		{ID: "shard-c", ChangedPaths: []string{"internal/c.go"}, Theories: []QATheory{{ID: "theory-c1", ShardID: "shard-c"}}},
	}
	first, err := groupQATheories(qaMap, shards, 2)
	if err != nil {
		t.Fatal(err)
	}
	second, err := groupQATheories(qaMap, shards, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("theory grouping is not deterministic:\n%+v\n%+v", first, second)
	}
	groupByTheory := map[string]string{}
	for _, group := range first {
		if len(group.Theories) > 2 {
			t.Fatalf("group %s exceeded maximum: %+v", group.ID, group.Theories)
		}
		for _, theory := range group.Theories {
			groupByTheory[theory.ID] = group.ID
		}
	}
	if groupByTheory["theory-a1"] != groupByTheory["theory-a2"] || groupByTheory["theory-b1"] != groupByTheory["theory-b2"] {
		t.Fatalf("context-similar theories were split: %+v", groupByTheory)
	}
}

func TestDeterministicIssueReconciliationUnionsEvidenceAndStrongestSeverity(t *testing.T) {
	qaMap := QAMap{ID: "map"}
	issues := []QAArbiterIssue{
		{TheoryIDs: []string{"theory-a"}, Claim: "Shared cause", Title: "Z title", IssueClass: "logic", Severity: "medium", Location: "internal/a.go", Reason: "first", EvidenceRefs: []string{"evidence-a"}},
		{TheoryIDs: []string{"theory-b"}, Claim: " shared cause ", Title: "A title", IssueClass: "LOGIC", Severity: "critical", Location: "./internal/a.go", Reason: "second", EvidenceRefs: []string{"evidence-b"}},
	}
	got := deterministicQAArbiterIssueReconciliation(qaMap, issues)
	if len(got) != 1 {
		t.Fatalf("reconciled issues = %+v", got)
	}
	if got[0].Severity != "critical" || got[0].Title != "A title" || !reflect.DeepEqual(got[0].TheoryIDs, []string{"theory-a", "theory-b"}) || !reflect.DeepEqual(got[0].EvidenceRefs, []string{"evidence-a", "evidence-b"}) {
		t.Fatalf("reconciled issue lost strongest fields or evidence: %+v", got[0])
	}
}

func TestIssueReconcilerRejectsRepeatedTheoryMembership(t *testing.T) {
	provisional := []QAArbiterIssue{{TheoryIDs: []string{"theory-a"}, EvidenceRefs: []string{"evidence-a"}}}
	output := qaIssueReconcilerOutput{SchemaVersion: QASchemaVersion, Issues: []QAArbiterIssue{
		{TheoryIDs: []string{"theory-a"}, Claim: "one", Title: "one", IssueClass: "logic", Severity: "high", Location: "a.go", Reason: "one", EvidenceRefs: []string{"evidence-a"}},
		{TheoryIDs: []string{"theory-a"}, Claim: "two", Title: "two", IssueClass: "logic", Severity: "high", Location: "b.go", Reason: "two", EvidenceRefs: []string{"evidence-a"}},
	}}
	if _, err := validateQAReconcilerOutput(QAMap{ID: "map"}, provisional, output); err == nil {
		t.Fatal("reconciler accepted one theory in two issues")
	}
}

func TestIssueReconcilerFailsClosedForReferenceAndCoverageViolations(t *testing.T) {
	provisional := []QAArbiterIssue{
		{TheoryIDs: []string{"theory-a"}, EvidenceRefs: []string{"evidence-a"}},
		{TheoryIDs: []string{"theory-b"}, EvidenceRefs: []string{"evidence-b"}},
	}
	validIssue := func(theories, evidence []string) QAArbiterIssue {
		return QAArbiterIssue{TheoryIDs: theories, Claim: "claim", Title: "title", IssueClass: "logic", Severity: "high", Location: "a.go", Reason: "reason", EvidenceRefs: evidence}
	}
	cases := map[string]qaIssueReconcilerOutput{
		"unknown theory":    {SchemaVersion: QASchemaVersion, Issues: []QAArbiterIssue{validIssue([]string{"theory-a", "theory-unknown"}, []string{"evidence-a"})}},
		"omitted theory":    {SchemaVersion: QASchemaVersion, Issues: []QAArbiterIssue{validIssue([]string{"theory-a"}, []string{"evidence-a"})}},
		"invented evidence": {SchemaVersion: QASchemaVersion, Issues: []QAArbiterIssue{validIssue([]string{"theory-a", "theory-b"}, []string{"evidence-invented"})}},
	}
	for name, output := range cases {
		t.Run(name, func(t *testing.T) {
			if issues, err := validateQAReconcilerOutput(QAMap{ID: "map"}, provisional, output); err == nil || len(issues) != 0 {
				t.Fatalf("invalid reconciler output produced issues: %+v err=%v", issues, err)
			}
		})
	}
}

func TestIssueReconcilerStrictJSONRejectsMalformedUnknownAndMultipleValues(t *testing.T) {
	for name, content := range map[string]string{
		"malformed":       `{"schema_version":1`,
		"unknown field":   `{"schema_version":1,"issues":[],"invented":true}`,
		"multiple values": `{"schema_version":1,"issues":[]} {"schema_version":1,"issues":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			var output qaIssueReconcilerOutput
			if err := decodeStrictQAJSON(content, &output); err == nil {
				t.Fatalf("strict decoder accepted %s output", name)
			}
		})
	}
}

func TestIssueReconciliationAgentFailsClosed(t *testing.T) {
	qaMap := QAMap{ID: "map", Foundation: &QAFoundation{Fingerprint: strings.Repeat("a", 64)}, Budgets: DefaultQABudgets()}
	settings := QASettings{Runtime: StageRuntime{Model: "provider/base"}, Reconciler: StageRuntime{Model: "provider/reconciler", Variant: "high"}, Budgets: qaMap.Budgets}
	issues := []QAArbiterIssue{{TheoryIDs: []string{"theory-a"}, Claim: "one", Title: "one", IssueClass: "logic", Severity: "high", Location: "a.go", Reason: "one", EvidenceRefs: []string{"evidence-a"}}}
	service := NewService(t.TempDir()).WithRuntime(qaEmptyTheoryRuntime{}).WithQASettings(settings)
	if got, _, err := service.reconcileQAArbiterIssues(context.Background(), qaMap, issues, t.TempDir(), settings); err == nil || len(got) != 0 {
		t.Fatalf("invalid reconciler output produced deterministic semantic issues: %+v err=%v", got, err)
	}
}

func TestQAArbiterOutputContractNamesEveryAcceptedAction(t *testing.T) {
	for _, action := range []QAArbiterAction{QAArbiterConfirm, QAArbiterRefute, QAArbiterReplace, QAArbiterMerge, QAArbiterSplit, QAArbiterInvalidate, QAArbiterKeepInconclusive} {
		if !strings.Contains(qaArbiterOutputContract, `"`+string(action)+`"`) {
			t.Fatalf("arbiter output contract omits accepted action %q", action)
		}
	}
	if strings.Contains(qaArbiterOutputContract, `"retain"`) {
		t.Fatal("arbiter output contract advertises unsupported retain action")
	}
	if !strings.Contains(qaArbiterOutputContract, "at most one override") || !strings.Contains(qaArbiterOutputContract, "do not also emit separate overrides") {
		t.Fatal("arbiter output contract does not forbid duplicate supersession")
	}
	for _, required := range []string{`first output byte must be "{"`, `last output byte must be "}"`, "Do not emit Markdown fences"} {
		if !strings.Contains(qaArbiterOutputContract, required) {
			t.Fatalf("arbiter output contract omits framing rule %q", required)
		}
	}
}
