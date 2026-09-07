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
	byShard := map[string]int{}
	for i := range shards {
		byShard[shards[i].ID] = i
	}
	var publications []QATestPublication
	progressed := false
	rounds := qaEvidenceRoundsUsed(shards, requests)
	allTests := func() []QATestPublication { return mergeQATestPublications(retained, publications) }
	persist := func() error { return checkpoint(shards, allTests(), requests) }
	// Give never-authored claims a turn before revisions, regardless of hash order.
	order := make([]int, 0, len(requests))
	for i := range requests {
		if active[requests[i].ID] {
			order = append(order, i)
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		return qaRequestAuthoringUsed(requests[order[i]], requests) < qaRequestAuthoringUsed(requests[order[j]], requests)
	})
	for _, requestIndex := range order {
		request := &requests[requestIndex]
		if request.SupersededBy != "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			return shards, publications, progressed, err
		}
		previousTest := qaLatestRequestBundle(allTests(), *request)
		if previousTest != nil && qaUsableReproduction(qaMap, *previousTest) {
			request.Status, request.ReasonCode = "evidence_recorded", ""
			request.TestBundleID, request.LatestRunID = previousTest.Bundle.ID, previousTest.Runs[len(previousTest.Runs)-1].ID
			continue
		}
		index, ok := byShard[request.OriginShardID]
		if !ok {
			stopQAEvidenceRequest(request, "origin_shard_unavailable")
			continue
		}
		shard := &shards[index]
		// A retained bundle interrupted before execution, or blocked by infrastructure,
		// is executed verbatim. Authoring and execution have independent reservations.
		reuse := previousTest != nil && (len(previousTest.Runs) == 0 || qaTestInfrastructureBlocked(*previousTest))
		var publication QATestPublication
		if reuse {
			publication = *previousTest
			publication.Runs = append([]QAReproductionRun(nil), previousTest.Runs...)
			if len(request.ExecutionAttempts) == 0 {
				for _, run := range publication.Runs {
					completed := run.CompletedAt
					request.ExecutionAttempts = append(request.ExecutionAttempts, QAEvidenceExecutionAttempt{Number: len(request.ExecutionAttempts) + 1, Phase: "retained_execution", TestBundleID: publication.Bundle.ID, StartedAt: completed.Add(-run.Result.Duration), CompletedAt: &completed, RunID: run.ID, Failure: run.Failure})
				}
			}
		} else {
			if len(shard.Attempts) == 0 {
				stopQAEvidenceRequest(request, "original_session_unavailable")
				continue
			}
			if request.AccountingVersion < 2 && rounds[shard.ID] >= qaMap.Budgets.EvidenceRoundsPerShard+request.RecoveryAllowance {
				stopQAEvidenceRequest(request, "evidence_round_budget_exhausted")
				continue
			}
			if request.AccountingVersion >= 2 && qaAuthoringPoolExhausted(qaMap, shards, requests) {
				stopQAEvidenceRequest(request, "evidence_authoring_budget_exhausted")
				continue
			}
			if request.AccountingVersion >= 2 && qaRequestAuthoringUsed(*request, requests) >= qaMap.Budgets.TestsPerTheory+request.RecoveryAllowance {
				stopQAEvidenceRequest(request, "tests_per_theory_budget_exhausted")
				continue
			}
			if request.PreparationAttempts >= request.Attempts+3+request.RecoveryAllowance {
				stopQAEvidenceRequest(request, "infrastructure_retry_budget_exhausted")
				continue
			}
			evidenceBefore := qaShardEvidenceFingerprint(*shard)
			retryPrerequisite := request.ReasonCode == "original_session_unavailable" || request.ReasonCode == "investigator_workspace_unavailable" || request.RecoveryAllowance > 0 || request.Failure != nil && request.Failure.Retryable
			if request.Attempts > 0 && request.EvidenceFingerprint == evidenceBefore && request.Status != "running" && !retryPrerequisite {
				stopQAEvidenceRequest(request, "repeated_evidence_request_without_new_evidence")
				continue
			}
			request.PreparationAttempts++
			request.Status = "preparing"
			if err := persist(); err != nil {
				return shards, publications, progressed, err
			}
			spec, err := buildQARequestedReproductionSpec(qaMap, *shard, *request, target, s.now().UTC())
			if err != nil {
				recordQAEvidenceFailure(request, "specification", "reproduction_spec_unavailable", err)
				continue
			}
			if previousTest != nil {
				spec = previousTest.Spec
			}
			workspace := qaInvestigatorWorkspacePath(s.root, qaMap.SemanticAttemptID, shard.ID)
			if err := restoreQAInvestigatorEvidenceWorkspace(ctx, s.root, target, qaMap, *shard, allTests()); err != nil {
				recordQAEvidenceFailure(request, "workspace", "investigator_workspace_unavailable", err)
				continue
			}
			initial, err := s.QAInvestigatorRequest(qaMap, *shard, workspace)
			if err != nil {
				recordQAEvidenceFailure(request, "authoring", "original_session_unavailable", err)
				continue
			}
			original := shard.Attempts[0]
			initial.Provider, initial.Model = original.Provider, original.Model
			initial.Metadata["variant"], initial.RuntimeStorePath = original.Variant, original.RuntimeStoreRef
			var previousRun *QAReproductionRun
			if previousTest != nil && len(previousTest.Runs) > 0 {
				previousRun = &previousTest.Runs[len(previousTest.Runs)-1]
			}
			round := request.Attempts + 1
			if request.AccountingVersion < 2 {
				round = rounds[shard.ID] + 1
			}
			var checkpointErr error
			beforeStart := func() error {
				rounds[shard.ID]++
				request.Attempts++
				request.EvidenceRound, request.EvidenceFingerprint = rounds[shard.ID], evidenceBefore
				request.Status, request.ReasonCode, request.Failure = "running", "", nil
				checkpointErr = persist()
				return checkpointErr
			}
			_, files, attempt, continueErr := s.continueQAInvestigatorForEvidence(ctx, qaMap, *shard, initial, original, *request, spec, previousRun, round, beforeStart)
			if checkpointErr != nil {
				return shards, publications, progressed, checkpointErr
			}
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
				phase := "authoring"
				if attempt.Number == 0 && strings.Contains(continueErr.Error(), "snapshot") {
					phase = "workspace"
				}
				recordQAEvidenceFailure(request, phase, reason, continueErr)
				continue
			}
			bundle, err := BuildQATestBundle(qaMap.Project, qaMap.Sprint, spec, files, "", qaMap.Budgets)
			if err != nil {
				recordQAEvidenceFailure(request, "fixture", "test_bundle_invalid", err)
				continue
			}
			publication = QATestPublication{Spec: spec, Bundle: bundle, AuthoringAttempts: []QAInvestigatorAttempt{attempt}}
			publications = mergeQATestPublications(publications, []QATestPublication{publication})
			request.TestBundleID, request.Status = bundle.ID, "ready"
			// The executable bundle survives a crash even if no run was ever produced.
			if err := persist(); err != nil {
				return shards, publications, progressed, err
			}
		}
		for {
			for i := range request.ExecutionAttempts {
				abandoned := &request.ExecutionAttempts[i]
				if abandoned.CompletedAt == nil {
					stopped := s.now().UTC()
					abandoned.CompletedAt = &stopped
					abandoned.Failure = &QAFailureDiagnostic{Phase: "execution", Code: "execution_interrupted", Retryable: true, Diagnostic: "The prior owner stopped before recording an execution result."}
				}
			}
			count := 0
			for _, attempt := range request.ExecutionAttempts {
				if attempt.TestBundleID == publication.Bundle.ID {
					count++
				}
			}
			if count >= 3+request.RecoveryAllowance {
				stopQAEvidenceRequest(request, "infrastructure_retry_budget_exhausted")
				break
			}
			if count > 0 {
				select {
				case <-ctx.Done():
					return shards, publications, progressed, ctx.Err()
				case <-time.After(time.Duration(1<<min(count, 3)) * 100 * time.Millisecond):
				}
				request.InfrastructureRetries++
			}
			request.ExecutionAttempts = append(request.ExecutionAttempts, QAEvidenceExecutionAttempt{Number: len(request.ExecutionAttempts) + 1, Phase: "execution", TestBundleID: publication.Bundle.ID, StartedAt: s.now().UTC()})
			execution := &request.ExecutionAttempts[len(request.ExecutionAttempts)-1]
			request.Status = "executing"
			if err := persist(); err != nil {
				return shards, publications, progressed, err
			}
			parent, err := qaRuntimeTemp("ultraplan-qa-authored-test-")
			var run QAReproductionRun
			if err == nil {
				run, err = RunQAReproduction(ctx, QAReproductionRequest{Project: qaMap.Project, Sprint: qaMap.Sprint, TargetRoot: target, WorkspaceParent: parent, ProtectedRoots: []string{s.root, target}, Spec: publication.Spec, Bundle: publication.Bundle, Budgets: qaMap.Budgets, ExpectedTargetID: publication.Spec.ImplementationFingerprint, Runner: s.processRunner, Now: s.now})
				_ = os.RemoveAll(parent)
			}
			completed := s.now().UTC()
			execution.CompletedAt = &completed
			if err != nil {
				recordQAEvidenceFailure(request, "workspace", "reproduction_run_unavailable", err)
				execution.Failure = request.Failure
			} else {
				execution.RunID, execution.Failure = run.ID, run.Failure
				publication.Runs = append(publication.Runs, run)
				publications = mergeQATestPublications(publications, []QATestPublication{publication})
				request.TestBundleID, request.LatestRunID, request.Failure = publication.Bundle.ID, run.ID, run.Failure
				if run.Outcome == QAEvidenceFail || run.Outcome == QAEvidencePass {
					request.Status, request.ReasonCode = "evidence_recorded", ""
					request.NextAction = "Return the recorded assertions, controls and observations to arbitration."
				} else {
					stopQAEvidenceRequest(request, run.ReasonCode)
				}
				// Infrastructure failures do not add redundant evidence or consume a theory's test budget.
				if run.Failure == nil || !run.Failure.Retryable {
					progressed = applyQAReproductionToTheories(shard, *request, publication.Bundle, run) || progressed
				}
			}
			if err := persist(); err != nil {
				return shards, publications, progressed, err
			}
			if request.Failure == nil || !request.Failure.Retryable {
				break
			}
			if err := ctx.Err(); err != nil {
				return shards, publications, progressed, err
			}
		}
	}
	return shards, publications, progressed, persist()
}

func recordQAEvidenceFailure(request *QAArbiterEvidenceRequest, phase, reason string, err error) {
	request.Failure = qaFailureDiagnostic(phase, err, "")
	var execution *qaExecutionError
	if errors.As(err, &execution) {
		request.Failure = execution.Failure
	}
	if request.Failure.Retryable {
		reason = request.Failure.Code
	}
	stopQAEvidenceRequest(request, reason)
	request.NextAction += " " + request.Failure.Diagnostic
}

func qaRequestAuthoringUsed(request QAArbiterEvidenceRequest, requests []QAArbiterEvidenceRequest) int {
	maximum := 0
	for _, id := range request.TheoryIDs {
		count := 0
		for _, other := range requests {
			if containsQAString(other.TheoryIDs, id) {
				count += other.Attempts
			}
		}
		if count > maximum {
			maximum = count
		}
	}
	return maximum
}

func qaTestInfrastructureBlocked(test QATestPublication) bool {
	if len(test.Runs) == 0 {
		return true
	}
	run := test.Runs[len(test.Runs)-1]
	if run.Outcome != QAEvidenceInconclusive || !run.Cleanup.Complete || run.TargetIdentity != test.Spec.ImplementationFingerprint {
		return false
	}
	if run.Failure != nil {
		return run.Failure.Retryable
	}
	// Legacy evidence can be retried only when its retained diagnostic supports it.
	output := run.Result.Stdout + "\n" + run.Result.Stderr
	phase := "compile"
	if len(qaTestEvents(output)) > 0 {
		phase = "assertion"
	}
	failure := qaFailureDiagnostic(phase, nil, output)
	return failure.Retryable
}

func mergeQATestPublications(current, next []QATestPublication) []QATestPublication {
	result := append([]QATestPublication(nil), current...)
	for _, test := range next {
		index := -1
		for i := range result {
			if result[i].Bundle.ID == test.Bundle.ID {
				index = i
				break
			}
		}
		if index < 0 {
			result = append(result, test)
			continue
		}
		merged := result[index]
		for _, run := range test.Runs {
			found := false
			for _, old := range merged.Runs {
				if old.ID == run.ID {
					found = true
					break
				}
			}
			if !found {
				merged.Runs = append(append([]QAReproductionRun(nil), merged.Runs...), run)
			}
		}
		result[index] = merged
	}
	return result
}

func qaLatestRequestBundle(tests []QATestPublication, request QAArbiterEvidenceRequest) *QATestPublication {
	var latest *QATestPublication
	for i := range tests {
		test := &tests[i]
		if !qaTestAnswersRequest(*test, request) {
			continue
		}
		if test.Bundle.ID == request.TestBundleID {
			return test
		}
		if latest == nil || test.Spec.FrozenAt.After(latest.Spec.FrozenAt) {
			latest = test
		}
	}
	return latest
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

func qaAuthoringPoolExhausted(qaMap QAMap, shards []QAShard, requests []QAArbiterEvidenceRequest) bool {
	theories, used, allowance := 0, 0, 0
	for _, shard := range shards {
		theories += len(shard.Theories)
	}
	for _, request := range requests {
		used += request.Attempts
		allowance += request.RecoveryAllowance
	}
	return used >= max(theories, len(shards)*qaMap.Budgets.EvidenceRoundsPerShard)+allowance
}
