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

func (s Service) strengthenQARequestedEvidence(ctx context.Context, qaMap QAMap, target string, shards []QAShard, requests []QAArbiterEvidenceRequest, rounds map[string]int, seen map[string]string) ([]QAShard, []QATestPublication, bool) {
	byShard := make(map[string]int, len(shards))
	for i := range shards {
		byShard[shards[i].ID] = i
	}
	var publications []QATestPublication
	progressed := false
	for _, request := range requests {
		index, ok := byShard[request.OriginShardID]
		if !ok {
			continue
		}
		shard := &shards[index]
		if len(shard.Attempts) == 0 {
			markQARequestInconclusive(shard, request, "original_session_unavailable")
			continue
		}
		if rounds[shard.ID] >= qaMap.Budgets.EvidenceRoundsPerShard {
			markQARequestInconclusive(shard, request, "evidence_round_budget_exhausted")
			continue
		}
		if qaTheoryTestBudgetExhausted(*shard, request.TheoryIDs, qaMap.Budgets.TestsPerTheory) {
			markQARequestInconclusive(shard, request, "tests_per_theory_budget_exhausted")
			continue
		}
		evidenceBefore := qaShardEvidenceFingerprint(*shard)
		if seen[request.ID] == evidenceBefore {
			markQARequestInconclusive(shard, request, "repeated_evidence_request_without_new_evidence")
			continue
		}
		seen[request.ID] = evidenceBefore
		rounds[shard.ID]++
		spec, err := buildQARequestedReproductionSpec(qaMap, *shard, request, target, s.now().UTC())
		if err != nil {
			markQARequestInconclusive(shard, request, "reproduction_spec_unavailable")
			continue
		}
		workspace := qaInvestigatorWorkspacePath(s.root, qaMap.SemanticAttemptID, shard.ID)
		initial, err := s.QAInvestigatorRequest(qaMap, *shard, workspace)
		if err != nil {
			markQARequestInconclusive(shard, request, "original_session_unavailable")
			continue
		}
		original := shard.Attempts[0]
		initial.Provider, initial.Model = original.Provider, original.Model
		initial.Metadata["variant"], initial.RuntimeStorePath = original.Variant, original.RuntimeStoreRef
		result, files, attempt, continueErr := s.continueQAInvestigatorForEvidence(ctx, qaMap, *shard, initial, original, request, spec, nil, rounds[shard.ID])
		if attempt.Number > 0 {
			shard.Attempts = append(shard.Attempts, attempt)
		}
		if continueErr != nil {
			reason := "evidence_authoring_inconclusive"
			if strings.Contains(continueErr.Error(), "original_session_unavailable") {
				reason = "original_session_unavailable"
			}
			markQARequestInconclusive(shard, request, reason)
			continue
		}
		_ = result
		bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, files, "", qaMap.Budgets)
		if err != nil {
			markQARequestInconclusive(shard, request, "test_bundle_invalid")
			continue
		}
		workspaceParent, err := os.MkdirTemp("", "ultraplan-qa-authored-test-")
		if err != nil {
			markQARequestInconclusive(shard, request, "reproduction_workspace_unavailable")
			continue
		}
		run, runErr := RunQAReproduction(ctx, QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: workspaceParent, ProtectedRoots: []string{s.root, target}, Spec: spec, Bundle: bundle, Budgets: qaMap.Budgets, ExpectedTargetID: spec.ImplementationFingerprint, Now: s.now})
		_ = os.RemoveAll(workspaceParent)
		if runErr != nil {
			markQARequestInconclusive(shard, request, "reproduction_run_unavailable")
			continue
		}
		publication := QATestPublication{Spec: spec, Bundle: bundle, AuthoringAttempts: []QAInvestigatorAttempt{attempt}, Runs: []QAReproductionRun{run}}
		publications = append(publications, publication)
		progressed = applyQAReproductionToTheories(shard, request, bundle, run) || progressed
	}
	return shards, publications, progressed
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
	command := QACheckDescriptor{ID: "investigator-test-" + strings.ToLower(suffix), Executable: executable, Args: []string{"test", ".", "-run", "^" + testName + "$", "-count=1"}, Environment: []string{"PATH"}, WorkingDirectory: filepath.ToSlash(filepath.Dir(source)), Timeout: qaMap.Budgets.CommandTimeout, OutputLimit: qaMap.Budgets.CommandOutputBytes}
	command.Fingerprint, err = fingerprintQAValue(command)
	if err != nil {
		return QAReproductionSpec{}, err
	}
	spec := QAReproductionSpec{AttemptID: qaMap.SemanticAttemptID, ShardID: shard.ID, TheoryIDs: append([]string(nil), request.TheoryIDs...), Claim: request.Gap, Preconditions: []string{request.ControlRequirement}, ExpectedBehavior: request.RequestedEvidence, PredictedFailure: QAFailureSignature{TestName: testName, Assertion: assertion, ExitClass: "nonzero", OutputMatcher: matcher}, InconclusiveConditions: []string{"compile error", "unrelated panic", "timeout", "truncated output", "infrastructure error", "mismatched assertion"}, ApprovedTestPaths: []string{testPath}, Command: command, ImplementationFingerprint: qaMap.ImplementationFingerprint}
	return FreezeQAReproductionSpec(qaMap.Project, qaMap.Sprint, spec, qaMap.Budgets, now)
}

func markQARequestInconclusive(shard *QAShard, request QAArbiterEvidenceRequest, reason string) {
	wanted := stringSet(request.TheoryIDs)
	for i := range shard.Theories {
		if wanted[shard.Theories[i].ID] {
			shard.Theories[i].Outcome = QATheoryInconclusive
			shard.Theories[i].OutcomeReason = reason
			shard.Theories[i].AttemptHistory = append([]QAInvestigatorAttempt(nil), shard.Attempts...)
		}
	}
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
		theory.Evidence = append(theory.Evidence, QAEvidenceSummary{Kind: "investigator_test", Summary: run.ReasonCode, Paths: testBundlePaths(bundle), CheckID: bundle.ID, OutputDigest: run.Result.StdoutDigest})
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
	for i := range requests {
		request := &requests[i]
		request.Status, request.NextAction = "pending", "Continue the original investigator session."
		for _, test := range tests {
			if test.Spec.ShardID != request.OriginShardID || sharedQAStrings(test.Spec.TheoryIDs, request.TheoryIDs) != len(request.TheoryIDs) || len(test.Runs) == 0 {
				continue
			}
			request.Status = "evidence_recorded"
			request.EvidenceRound = len(test.AuthoringAttempts)
			request.TestBundleID = test.Bundle.ID
			request.LatestRunID = test.Runs[len(test.Runs)-1].ID
			request.NextAction = "Inspect the retained reproduction run and arbitration outcome."
		}
		if request.Status != "pending" {
			continue
		}
		for _, shard := range shards {
			if shard.ID != request.OriginShardID {
				continue
			}
			wanted := stringSet(request.TheoryIDs)
			for _, theory := range shard.Theories {
				if wanted[theory.ID] && strings.Contains(theory.OutcomeReason, "unavailable") || wanted[theory.ID] && strings.Contains(theory.OutcomeReason, "exhausted") || wanted[theory.ID] && strings.Contains(theory.OutcomeReason, "repeated_evidence_request") {
					request.Status, request.NextAction = "inconclusive", theory.OutcomeReason
				}
			}
		}
	}
	return requests
}
