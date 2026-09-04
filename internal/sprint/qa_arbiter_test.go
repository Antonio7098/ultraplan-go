package sprint

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type retainedArbiterRuntime struct {
	requests            []pruntime.Request
	replaceContinuation bool
	theoryID            string
	shardID             string
}

func (runtime *retainedArbiterRuntime) StartRun(_ context.Context, req pruntime.Request) (pruntime.Result, error) {
	runtime.requests = append(runtime.requests, req)
	output := qaArbiterOutput{SchemaVersion: QASchemaVersion}
	if req.SessionAction == "continue" {
		output.Overrides = []QAArbiterOverride{{TheoryIDs: []string{runtime.theoryID}, Action: QAArbiterConfirm, Outcome: QATheoryConfirmed, Reason: "the returned test now discriminates the claim", ReasonRefs: []string{runtime.theoryID}, Confidence: 0.9}}
		output.Issues = []QAArbiterIssue{{TheoryIDs: []string{runtime.theoryID}, Claim: "confirmed defect", Title: "Confirmed defect", IssueClass: "behavior", Severity: "high", Location: "internal/app/usecases.go", Reason: "the authored test reproduces the claim", EvidenceRefs: []string{runtime.theoryID}}}
	} else {
		output.Overrides = []QAArbiterOverride{{TheoryIDs: []string{runtime.theoryID}, Action: QAArbiterKeepInconclusive, Outcome: QATheoryInconclusive, Reason: "a discriminating test is still required", ReasonRefs: []string{runtime.theoryID}, Confidence: 0.5}}
		output.EvidenceRequests = []QAArbiterEvidenceRequest{{TheoryIDs: []string{runtime.theoryID}, OriginShardID: runtime.shardID, Gap: "no executable reproducer", RequestedEvidence: "a focused regression test", RequiredObservation: "the valid input is rejected", ControlRequirement: "include a valid control", Priority: "high"}}
	}
	data, _ := json.Marshal(output)
	sessionID := "arbiter-session"
	if req.SessionAction == "continue" && runtime.replaceContinuation {
		sessionID = "replacement-session"
	}
	return pruntime.Result{Status: "completed", SessionID: sessionID, RuntimeStorePath: req.RuntimeStorePath, TerminalOutput: string(data), Permissions: pruntime.PermissionSummary{Mode: "restricted", Default: "deny"}}, nil
}

func TestEvidenceReturnResumesExactArbiterSession(t *testing.T) {
	root, sp, target, qaMap, _, _, token := qaRunFixture(t)
	qaMap.Foundation = &QAFoundation{ID: "foundation", Fingerprint: strings.Repeat("f", 64)}
	shard := qaMap.Shards[0]
	theoryID, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, QATheoryIdentity{Claim: "valid input is rejected", Basis: "changed branch", VerificationSurface: "internal/app/usecases.go"})
	if err != nil {
		t.Fatal(err)
	}
	shard.Theories = []QATheory{{ID: theoryID, ShardID: shard.ID, Claim: "valid input is rejected", Outcome: QATheoryInconclusive}}
	groups, err := groupQATheories(qaMap, []QAShard{shard}, qaMap.Budgets.ArbiterMaxTheories)
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	runtime := &retainedArbiterRuntime{theoryID: theoryID, shardID: shard.ID}
	settings := QASettings{Runtime: StageRuntime{Model: "provider/base"}, Arbiter: StageRuntime{Model: "provider/arbiter", Variant: "high"}, Budgets: qaMap.Budgets}
	service := NewService(root).WithRuntime(runtime).WithQASettings(settings)
	first, err := service.runQAArbiterGroup(context.Background(), qaMap, groups[0], target, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.SessionID != "arbiter-session" || first.Provider != "provider" || first.Model != "provider/arbiter" || first.Variant != "high" || first.RuntimeStoreRef == "" || first.WorkspaceID == "" || first.Round != 1 {
		t.Fatalf("initial arbiter identity = %+v", first)
	}
	if len(first.EvidenceRequests) != 1 || first.EvidenceRequests[0].ArbiterGroupID != first.ID {
		t.Fatalf("evidence request lost its arbiter binding: %+v", first.EvidenceRequests)
	}
	second, err := service.runQAArbiterGroup(context.Background(), qaMap, groups[0], target, settings, &first)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.requests) != 2 || runtime.requests[1].SessionID != first.SessionID || runtime.requests[1].SessionAction != "continue" || runtime.requests[1].RuntimeStorePath != first.RuntimeStoreRef || second.SessionID != first.SessionID || second.Round != 2 {
		t.Fatalf("arbiter continuation = requests %+v, group %+v", runtime.requests, second)
	}
	store := NewQAStore(root, sp).WithWriterFence(func(got QAWriterToken) error {
		if got != token {
			return context.Canceled
		}
		return nil
	})
	if err := store.PublishArbiterSessionGroups(qaMap.SemanticAttemptID, []QAArbiterGroup{first}, token); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishArbiterSessionGroups(qaMap.SemanticAttemptID, []QAArbiterGroup{second}, token); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadLatestArbiterSessionGroups(qaMap.SemanticAttemptID)
	if err != nil || len(loaded) != 1 || loaded[0].SessionID != first.SessionID || loaded[0].Round != 2 {
		t.Fatalf("loaded arbiter session = %+v, %v", loaded, err)
	}
}

func TestEvidenceReturnRestoresRetainedArbiterStoreBeforeIdentityValidation(t *testing.T) {
	root, _, target, qaMap, _, _, _ := qaRunFixture(t)
	qaMap.Foundation = &QAFoundation{ID: "foundation", Fingerprint: strings.Repeat("f", 64)}
	shard := qaMap.Shards[0]
	theoryID, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, QATheoryIdentity{Claim: "valid input is rejected", Basis: "changed branch", VerificationSurface: "internal/app/usecases.go"})
	if err != nil {
		t.Fatal(err)
	}
	shard.Theories = []QATheory{{ID: theoryID, ShardID: shard.ID, Claim: "valid input is rejected", Outcome: QATheoryInconclusive}}
	groups, err := groupQATheories(qaMap, []QAShard{shard}, qaMap.Budgets.ArbiterMaxTheories)
	if err != nil || len(groups) != 1 {
		t.Fatalf("groups = %+v, %v", groups, err)
	}
	runtime := &retainedArbiterRuntime{theoryID: theoryID, shardID: shard.ID}
	settings := QASettings{Runtime: StageRuntime{Model: "provider/base"}, Arbiter: StageRuntime{Model: "provider/arbiter", Variant: "high"}, Budgets: qaMap.Budgets}
	service := NewService(root).WithRuntime(runtime).WithQASettings(settings)
	previous := QAArbiterGroup{
		ID: groups[0].ID, TheoryIDs: []string{theoryID}, SessionID: "arbiter-session",
		Provider: "provider", Model: "provider/arbiter", Variant: "high",
		RuntimeStoreRef: "/retained/runtime/opencode.db", WorkspaceID: hashOpaque(target), Round: 1,
	}
	continued, err := service.runQAArbiterGroup(context.Background(), qaMap, groups[0], target, settings, &previous)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.requests) != 1 || runtime.requests[0].RuntimeStorePath != previous.RuntimeStoreRef || continued.SessionID != previous.SessionID || continued.Round != 2 {
		t.Fatalf("arbiter continuation did not restore retained store: requests %+v, group %+v", runtime.requests, continued)
	}
}

func TestEvidenceReturnRejectsReplacementArbiterSession(t *testing.T) {
	root, _, target, qaMap, _, _, _ := qaRunFixture(t)
	qaMap.Foundation = &QAFoundation{ID: "foundation", Fingerprint: strings.Repeat("f", 64)}
	shard := qaMap.Shards[0]
	theoryID, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, QATheoryIdentity{Claim: "valid input is rejected", Basis: "changed branch", VerificationSurface: "internal/app/usecases.go"})
	if err != nil {
		t.Fatal(err)
	}
	shard.Theories = []QATheory{{ID: theoryID, ShardID: shard.ID, Claim: "valid input is rejected", Outcome: QATheoryInconclusive}}
	groups, err := groupQATheories(qaMap, []QAShard{shard}, qaMap.Budgets.ArbiterMaxTheories)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &retainedArbiterRuntime{theoryID: theoryID, shardID: shard.ID, replaceContinuation: true}
	settings := QASettings{Runtime: StageRuntime{Model: "provider/base"}, Arbiter: StageRuntime{Model: "provider/arbiter", Variant: "high"}, Budgets: qaMap.Budgets}
	service := NewService(root).WithRuntime(runtime).WithQASettings(settings)
	first, err := service.runQAArbiterGroup(context.Background(), qaMap, groups[0], target, settings, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.runQAArbiterGroup(context.Background(), qaMap, groups[0], target, settings, &first); err == nil || !strings.Contains(err.Error(), "original_arbiter_session_unavailable") {
		t.Fatalf("replacement arbiter session error = %v", err)
	}
}

func TestArbiterPromptStatesExecutableEvidenceRequestBudget(t *testing.T) {
	root, _, target, qaMap, _, _, _ := qaRunFixture(t)
	qaMap.Foundation = &QAFoundation{ID: "foundation", Fingerprint: strings.Repeat("f", 64)}
	qaMap.Budgets.EvidenceRoundsPerShard = 2
	shard := qaMap.Shards[0]
	theoryID, err := NewQATheoryID(qaMap.Project, qaMap.Sprint, shard.ID, QATheoryIdentity{Claim: "valid input is rejected", Basis: "changed branch", VerificationSurface: "internal/app/usecases.go"})
	if err != nil {
		t.Fatal(err)
	}
	shard.Theories = []QATheory{{ID: theoryID, ShardID: shard.ID, Claim: "valid input is rejected", Outcome: QATheoryInconclusive}}
	groups, err := groupQATheories(qaMap, []QAShard{shard}, qaMap.Budgets.ArbiterMaxTheories)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &retainedArbiterRuntime{theoryID: theoryID, shardID: shard.ID}
	service := NewService(root).WithRuntime(runtime).WithQASettings(QASettings{Runtime: StageRuntime{Model: "provider/base"}, Arbiter: StageRuntime{Model: "provider/arbiter"}, Budgets: qaMap.Budgets})
	if _, err := service.runQAArbiterGroup(context.Background(), qaMap, groups[0], target, service.qaSettings, nil); err != nil {
		t.Fatal(err)
	}
	prompt := runtime.requests[0].Prompt
	for _, want := range []string{"at most 2 requests per origin shard", "executable Go _test.go reproducer", "Do not request another source excerpt", "only supplied theory IDs in reason_refs"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("arbiter prompt omitted %q", want)
		}
	}
}

func TestArbiterReferencesFallBackToTheirDeliveredTheories(t *testing.T) {
	output := qaArbiterOutput{
		Overrides: []QAArbiterOverride{{TheoryIDs: []string{"theory-a"}, ReasonRefs: []string{"invented-block"}}},
		Issues:    []QAArbiterIssue{{TheoryIDs: []string{"theory-a"}, EvidenceRefs: []string{"delivered-block", "invented-block"}}},
	}
	normalizeQAArbiterReferences(&output, map[string]bool{"theory-a": true, "delivered-block": true})
	if !reflect.DeepEqual(output.Overrides[0].ReasonRefs, []string{"theory-a"}) {
		t.Fatalf("override refs = %v", output.Overrides[0].ReasonRefs)
	}
	if !reflect.DeepEqual(output.Issues[0].EvidenceRefs, []string{"delivered-block", "theory-a"}) {
		t.Fatalf("issue refs = %v", output.Issues[0].EvidenceRefs)
	}
}

func TestArbiterDiscardsEvidenceRequestsResolvedBySameOutput(t *testing.T) {
	requests := []QAArbiterEvidenceRequest{
		{TheoryIDs: []string{"confirmed"}},
		{TheoryIDs: []string{"inconclusive"}},
		{TheoryIDs: []string{"inconclusive", "refuted"}},
	}
	outcomes := map[string]QATheoryOutcome{
		"confirmed":    QATheoryConfirmed,
		"inconclusive": QATheoryInconclusive,
		"refuted":      QATheoryRefuted,
	}
	got := discardResolvedQAArbiterEvidenceRequests(requests, outcomes)
	if len(got) != 1 || len(got[0].TheoryIDs) != 1 || got[0].TheoryIDs[0] != "inconclusive" {
		t.Fatalf("unexpected retained requests: %+v", got)
	}
}

func TestArbiterIssuesRetainOnlyConfirmedTheories(t *testing.T) {
	issues := []QAArbiterIssue{
		{TheoryIDs: []string{"confirmed", "inconclusive"}},
		{TheoryIDs: []string{"invalid"}},
	}
	outcomes := map[string]QATheoryOutcome{
		"confirmed":    QATheoryConfirmed,
		"inconclusive": QATheoryInconclusive,
		"invalid":      QATheoryInvalid,
	}
	got := retainConfirmedQAArbiterIssueTheories(issues, outcomes)
	if len(got) != 1 || len(got[0].TheoryIDs) != 1 || got[0].TheoryIDs[0] != "confirmed" {
		t.Fatalf("unexpected retained issues: %+v", got)
	}
}

func TestRetainedArbiterIdentityRejectsEveryRuntimeChange(t *testing.T) {
	workdir := t.TempDir()
	group := qaTheoryGroupPlan{ID: "qa-v1-arbiter-group-aaaaaaaaaaaaaaaaaaaaaaaa", Theories: []QATheory{{ID: "theory-a"}}}
	req := pruntime.Request{Provider: "provider", Model: "arbiter", WorkDir: workdir, RuntimeStorePath: "/runtime", Metadata: map[string]string{"variant": "high"}}
	previous := QAArbiterGroup{ID: group.ID, TheoryIDs: []string{"theory-a"}, SessionID: "session", Provider: "provider", Model: "provider/arbiter", Variant: "high", RuntimeStoreRef: "/runtime", WorkspaceID: hashOpaque(workdir), Round: 1}
	if err := validateRetainedQAArbiterIdentity(previous, group, req); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*QAArbiterGroup, *qaTheoryGroupPlan, *pruntime.Request){
		"session":  func(group *QAArbiterGroup, _ *qaTheoryGroupPlan, _ *pruntime.Request) { group.SessionID = "" },
		"provider": func(_ *QAArbiterGroup, _ *qaTheoryGroupPlan, req *pruntime.Request) { req.Provider = "replacement" },
		"model":    func(_ *QAArbiterGroup, _ *qaTheoryGroupPlan, req *pruntime.Request) { req.Model = "replacement" },
		"variant":  func(_ *QAArbiterGroup, _ *qaTheoryGroupPlan, req *pruntime.Request) { req.Metadata["variant"] = "low" },
		"store": func(_ *QAArbiterGroup, _ *qaTheoryGroupPlan, req *pruntime.Request) {
			req.RuntimeStorePath = "/replacement"
		},
		"workspace": func(_ *QAArbiterGroup, _ *qaTheoryGroupPlan, req *pruntime.Request) { req.WorkDir = t.TempDir() },
		"theories": func(_ *QAArbiterGroup, group *qaTheoryGroupPlan, _ *pruntime.Request) {
			group.Theories = []QATheory{{ID: "theory-b"}}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidateGroup, candidatePlan, candidateRequest := previous, group, req
			candidateRequest.Metadata = cloneMetadata(req.Metadata, nil)
			mutate(&candidateGroup, &candidatePlan, &candidateRequest)
			if err := validateRetainedQAArbiterIdentity(candidateGroup, candidatePlan, candidateRequest); err == nil {
				t.Fatal("changed arbiter identity was accepted")
			}
		})
	}
}

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
