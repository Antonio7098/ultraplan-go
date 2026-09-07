package sprint

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var qaTestNameCleaner = regexp.MustCompile(`[^A-Za-z0-9_]`)

func (s Service) strengthenQARequestedEvidence(ctx context.Context, qaMap QAMap, target string, shards []QAShard, requests []QAArbiterEvidenceRequest, active map[string]bool, retained []QATestPublication, checkpoint func([]QAShard, []QATestPublication, []QAArbiterEvidenceRequest) error) ([]QAShard, []QATestPublication, bool, error) {
	byShard := make(map[string]int, len(shards))
	for i := range shards {
		byShard[shards[i].ID] = i
	}
	var publications []QATestPublication
	progressed := false
	rounds := qaEvidenceRoundsUsed(shards, requests)
	persist := func() error {
		return checkpoint(shards, append(append([]QATestPublication(nil), retained...), publications...), requests)
	}
	for requestIndex := range requests {
		request := &requests[requestIndex]
		if !active[request.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return shards, publications, progressed, err
		}
		previousTest := qaLatestRequestTest(retained, *request)
		if previousTest != nil && qaUsableReproduction(qaMap, *previousTest) {
			request.Status, request.ReasonCode = "evidence_recorded", ""
			request.TestBundleID = previousTest.Bundle.ID
			request.LatestRunID = previousTest.Runs[len(previousTest.Runs)-1].ID
			continue
		}
		index, ok := byShard[request.OriginShardID]
		if !ok {
			stopQAEvidenceRequest(request, "origin_shard_unavailable")
			continue
		}
		shard := &shards[index]
		if len(shard.Attempts) == 0 {
			stopQAEvidenceRequest(request, "original_session_unavailable")
			continue
		}
		if rounds[shard.ID] >= qaMap.Budgets.EvidenceRoundsPerShard {
			stopQAEvidenceRequest(request, "evidence_round_budget_exhausted")
			continue
		}
		if qaTheoryTestBudgetExhausted(*shard, request.TheoryIDs, qaMap.Budgets.TestsPerTheory) {
			stopQAEvidenceRequest(request, "tests_per_theory_budget_exhausted")
			continue
		}
		evidenceBefore := qaShardEvidenceFingerprint(*shard)
		retryPrerequisite := request.ReasonCode == "original_session_unavailable" || request.ReasonCode == "investigator_workspace_unavailable" || request.ReasonCode == "reproduction_workspace_unavailable"
		if request.Attempts > 0 && request.EvidenceFingerprint == evidenceBefore && request.Status != "running" && !retryPrerequisite {
			stopQAEvidenceRequest(request, "repeated_evidence_request_without_new_evidence")
			continue
		}
		rounds[shard.ID]++
		request.Attempts++
		request.EvidenceRound, request.EvidenceFingerprint = rounds[shard.ID], evidenceBefore
		request.Status, request.ReasonCode = "running", ""
		// Reserve the attempt before any model call. Resume cannot reset budgets
		// when a process dies during authoring or execution.
		if err := persist(); err != nil {
			return shards, publications, progressed, err
		}
		spec, err := buildQARequestedReproductionSpec(qaMap, *shard, *request, target, s.now().UTC())
		if err != nil {
			stopQAEvidenceRequest(request, "reproduction_spec_unavailable")
			continue
		}
		if previousTest != nil {
			spec = previousTest.Spec
		}
		workspace := qaInvestigatorWorkspacePath(s.root, qaMap.SemanticAttemptID, shard.ID)
		if err := restoreQAInvestigatorEvidenceWorkspace(ctx, s.root, target, qaMap, *shard, retained); err != nil {
			stopQAEvidenceRequest(request, "investigator_workspace_unavailable")
			continue
		}
		initial, err := s.QAInvestigatorRequest(qaMap, *shard, workspace)
		if err != nil {
			stopQAEvidenceRequest(request, "original_session_unavailable")
			continue
		}
		original := shard.Attempts[0]
		initial.Provider, initial.Model = original.Provider, original.Model
		initial.Metadata["variant"], initial.RuntimeStorePath = original.Variant, original.RuntimeStoreRef
		var previousRun *QAReproductionRun
		if previousTest != nil {
			previousRun = &previousTest.Runs[len(previousTest.Runs)-1]
		}
		result, files, attempt, continueErr := s.continueQAInvestigatorForEvidence(ctx, qaMap, *shard, initial, original, *request, spec, previousRun, rounds[shard.ID])
		if attempt.Number > 0 {
			shard.Attempts = append(shard.Attempts, attempt)
		}
		if continueErr != nil {
			reason := "evidence_authoring_inconclusive"
			if strings.Contains(continueErr.Error(), "original_session_unavailable") {
				reason = "original_session_unavailable"
			} else if strings.Contains(attempt.StopReason, "verification_unavailable:") {
				reason = "verification_unavailable"
			}
			stopQAEvidenceRequest(request, reason)
			request.NextAction += " " + safeReportText(safeError(continueErr))
			continue
		}
		_ = result
		bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, files, "", qaMap.Budgets)
		if err != nil {
			stopQAEvidenceRequest(request, "test_bundle_invalid")
			continue
		}
		workspaceParent, err := os.MkdirTemp("", "ultraplan-qa-authored-test-")
		if err != nil {
			stopQAEvidenceRequest(request, "reproduction_workspace_unavailable")
			continue
		}
		run, runErr := RunQAReproduction(ctx, QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: workspaceParent, ProtectedRoots: []string{s.root, target}, Spec: spec, Bundle: bundle, Budgets: qaMap.Budgets, ExpectedTargetID: spec.ImplementationFingerprint, Now: s.now})
		_ = os.RemoveAll(workspaceParent)
		if runErr != nil {
			stopQAEvidenceRequest(request, "reproduction_run_unavailable")
			continue
		}
		publication := QATestPublication{Spec: spec, Bundle: bundle, AuthoringAttempts: []QAInvestigatorAttempt{attempt}, Runs: []QAReproductionRun{run}}
		publications = append(publications, publication)
		request.TestBundleID, request.LatestRunID = bundle.ID, run.ID
		if run.Outcome == QAEvidenceFail || run.Outcome == QAEvidencePass {
			request.Status, request.ReasonCode = "evidence_recorded", ""
			request.NextAction = "Return the recorded test and observations to arbitration."
		} else {
			stopQAEvidenceRequest(request, run.ReasonCode)
		}
		progressed = applyQAReproductionToTheories(shard, *request, bundle, run) || progressed
		if err := persist(); err != nil {
			return shards, publications, progressed, err
		}
	}
	return shards, publications, progressed, persist()
}

func buildQARequestedReproductionSpec(qaMap QAMap, shard QAShard, request QAArbiterEvidenceRequest, target string, now time.Time) (QAReproductionSpec, error) {
	var source string
	for _, candidate := range append(append([]string(nil), shard.ChangedPaths...), shard.ContextPaths...) {
		if strings.HasSuffix(candidate, ".go") && !strings.HasSuffix(candidate, "_test.go") {
			source = candidate
			break
		}
	}
	if source == "" {
		return QAReproductionSpec{}, errors.New("no Go source path is owned by the shard")
	}
	suffix := request.ID
	if len(suffix) > 12 {
		suffix = suffix[len(suffix)-12:]
	}
	testName := "TestQAInvestigator_" + qaTestNameCleaner.ReplaceAllString(suffix, "_")
	testPath := filepath.ToSlash(filepath.Join(filepath.Dir(source), "qa_investigator_"+strings.ToLower(suffix)+"_test.go"))
	executable, err := exec.LookPath("go")
	if err != nil {
		return QAReproductionSpec{}, err
	}
	assertion := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(request.RequiredObservation, "\r", " "), "\n", " "))
	if len(assertion) > 256 {
		assertion = assertion[:256]
	}
	if assertion == "" {
		return QAReproductionSpec{}, errors.New("required observation is empty")
	}
	// The output discriminator must be practical for an authored test to emit
	// verbatim. Arbiter prose remains the human-readable assertion, while this
	// stable marker prevents punctuation, quoting, and truncation differences
	// from turning a reproduced defect into a signature mismatch.
	matcher := "ULTRAPLAN_QA_PREDICTED_FAILURE:" + testName
	command := QACheckDescriptor{ID: "investigator-test-" + strings.ToLower(suffix), Executable: executable, Args: []string{"test", ".", "-v", "-run", "^" + testName + "$", "-count=1"}, Environment: []string{"PATH"}, WorkingDirectory: filepath.ToSlash(filepath.Dir(source)), Timeout: qaMap.Budgets.CommandTimeout, OutputLimit: qaMap.Budgets.CommandOutputBytes}
	command.Fingerprint, err = fingerprintQAValue(command)
	if err != nil {
		return QAReproductionSpec{}, err
	}
	var claims []string
	for _, theory := range shard.Theories {
		if containsQAString(request.TheoryIDs, theory.ID) && strings.TrimSpace(theory.Claim) != "" {
			claims = append(claims, theory.Claim)
		}
	}
	claim := strings.Join(claims, "; ")
	if claim == "" {
		claim = request.Gap
	}
	spec := QAReproductionSpec{AttemptID: qaMap.SemanticAttemptID, ShardID: shard.ID, EvidenceRequestID: request.ID, TheoryIDs: append([]string(nil), request.TheoryIDs...), Claim: claim, Preconditions: []string{request.ControlRequirement}, ExpectedBehavior: request.RequiredObservation, PredictedFailure: QAFailureSignature{TestName: testName, Assertion: assertion, ExitClass: "nonzero", OutputMatcher: matcher}, InconclusiveConditions: []string{"compile error", "unrelated panic", "timeout", "truncated output", "infrastructure error", "mismatched assertion"}, ApprovedTestPaths: []string{testPath}, Command: command, ImplementationFingerprint: qaMap.ImplementationFingerprint}
	return FreezeQAReproductionSpec(qaMap.Project, qaMap.Sprint, spec, qaMap.Budgets, now)
}

func applyQAReproductionToTheories(shard *QAShard, request QAArbiterEvidenceRequest, bundle QATestBundle, run QAReproductionRun) bool {
	wanted := stringSet(request.TheoryIDs)
	newEvidence := false
	for i := range shard.Theories {
		theory := &shard.Theories[i]
		if !wanted[theory.ID] {
			continue
		}
		duplicate := false
		for _, evidence := range theory.Evidence {
			if evidence.Kind == "investigator_test" && evidence.CheckID == bundle.ID && evidence.OutputDigest == run.Result.StdoutDigest {
				duplicate = true
				break
			}
		}
		if duplicate {
			theory.Outcome, theory.OutcomeReason = QATheoryInconclusive, "repeated_evidence_request_without_new_evidence"
			continue
		}
		newEvidence = true
		// Give the arbiter observations and actual assertions, not just the
		// classifier's verdict. Existing prompt budgets bound the full packet.
		summary := "Request " + request.ID + ": " + run.ReasonCode + "\nRequired observation: " + request.RequiredObservation
		for _, file := range bundle.Files {
			summary += "\nTest " + file.Path + ":\n" + file.Content
		}
		summary += "\nObserved stdout:\n" + run.Result.Stdout + "\nObserved stderr:\n" + run.Result.Stderr
		theory.Evidence = append(theory.Evidence, QAEvidenceSummary{Kind: "investigator_test", Summary: summary, Paths: testBundlePaths(bundle), CheckID: bundle.ID, OutputDigest: run.Result.StdoutDigest})
		theory.AttemptHistory = append([]QAInvestigatorAttempt(nil), shard.Attempts...)
		switch run.Outcome {
		case QAEvidenceFail:
			theory.Outcome, theory.OutcomeReason = QATheoryConfirmed, "investigator-authored test matched the frozen failure signature"
		case QAEvidencePass:
			theory.Outcome, theory.OutcomeReason = QATheoryRefuted, "investigator-authored test passed on the frozen implementation"
		default:
			theory.Outcome, theory.OutcomeReason = QATheoryInconclusive, run.ReasonCode
		}
	}
	return newEvidence
}

func qaTheoryTestBudgetExhausted(shard QAShard, theoryIDs []string, limit int) bool {
	wanted := stringSet(theoryIDs)
	for _, theory := range shard.Theories {
		if !wanted[theory.ID] {
			continue
		}
		count := 0
		for _, evidence := range theory.Evidence {
			if evidence.Kind == "investigator_test" {
				count++
			}
		}
		if count >= limit {
			return true
		}
	}
	return false
}

func testBundlePaths(bundle QATestBundle) []string {
	paths := make([]string, 0, len(bundle.Files))
	for _, file := range bundle.Files {
		paths = append(paths, file.Path)
	}
	return paths
}

func qaShardEvidenceFingerprint(shard QAShard) string {
	values := make([]string, 0)
	for _, theory := range shard.Theories {
		for _, evidence := range theory.Evidence {
			values = append(values, theory.ID+"\x00"+evidence.CheckID+"\x00"+evidence.OutputDigest)
		}
	}
	sort.Strings(values)
	value, _ := fingerprintQAValue(values)
	return value
}

func appendUniqueQAArbiterEvidenceRequests(current, next []QAArbiterEvidenceRequest) []QAArbiterEvidenceRequest {
	seen := make(map[string]bool, len(current)+len(next))
	for _, request := range current {
		seen[request.ID] = true
	}
	for _, request := range next {
		if !seen[request.ID] {
			current = append(current, request)
			seen[request.ID] = true
		}
	}
	sort.Slice(current, func(i, j int) bool { return current[i].ID < current[j].ID })
	return current
}

func finalizeQAArbiterEvidenceRequests(requests []QAArbiterEvidenceRequest, tests []QATestPublication, shards []QAShard) []QAArbiterEvidenceRequest {
	requests = append([]QAArbiterEvidenceRequest(nil), requests...)
	outcomes := map[string]QATheoryOutcome{}
	for _, shard := range shards {
		for _, theory := range shard.Theories {
			outcomes[theory.ID] = theory.Outcome
		}
	}
	for i := range requests {
		request := &requests[i]
		resolved := len(request.TheoryIDs) > 0
		for _, id := range request.TheoryIDs {
			switch outcomes[id] {
			case QATheoryRefuted, QATheoryInvalid, QATheoryNotApplicable:
			default:
				resolved = false
			}
		}
		if resolved {
			request.Status, request.ReasonCode, request.NextAction = "superseded", "theory_dismissed_by_arbitration", "Inspect the retained arbitration decision."
			continue
		}
		if test := qaLatestRequestTest(tests, *request); test != nil {
			run := test.Runs[len(test.Runs)-1]
			request.TestBundleID, request.LatestRunID = test.Bundle.ID, run.ID
			if ValidateQAReproductionRun(run, test.Spec, test.Bundle) == nil && run.TargetIdentity == test.Spec.ImplementationFingerprint && (run.Outcome == QAEvidenceFail || run.Outcome == QAEvidencePass) {
				request.Status, request.ReasonCode = "evidence_recorded", ""
				request.NextAction = "Inspect the retained reproduction run and arbitration outcome."
			} else if request.ReasonCode == "" {
				stopQAEvidenceRequest(request, "requested_observation_unresolved")
			}
		} else if request.Status == "evidence_recorded" {
			stopQAEvidenceRequest(request, "request_evidence_binding_missing")
		} else if request.Status == "" {
			request.Status, request.NextAction = "pending", "Continue the original investigator session."
		}
	}
	byID := map[string]int{}
	for i, request := range requests {
		byID[request.ID] = i
	}
	// Follow explicit forward lineage only. The exact request binding above
	// remains necessary for the replacement itself to be satisfied.
	for i := range requests {
		seen := map[string]bool{requests[i].ID: true}
		next := requests[i].SupersededBy
		for next != "" && !seen[next] {
			seen[next] = true
			index, ok := byID[next]
			if !ok {
				break
			}
			if requests[index].Status == "evidence_recorded" || requests[index].Status == "superseded" {
				requests[i].Status, requests[i].ReasonCode = "superseded", "replaced_by_answered_evidence_request"
				requests[i].NextAction = "Inspect replacement evidence request " + requests[i].SupersededBy + "."
				break
			}
			next = requests[index].SupersededBy
		}
	}
	return requests
}
