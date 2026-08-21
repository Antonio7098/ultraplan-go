package sprint

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
)

func (s Service) RunSmoke(ctx context.Context, projectRef, sprintRef string, req SmokeRequest) (result SmokeResult, err error) {
	if req.DryRun {
		return s.runSmoke(ctx, projectRef, sprintRef, req)
	}
	lockedCtx, release, lockErr := s.acquireMutationContext(ctx, projectRef, sprintRef)
	if lockErr != nil {
		return SmokeResult{Project: projectRef, Sprint: sprintRef, Status: SmokeFailed}, lockErr
	}
	defer release()
	ctx = lockedCtx
	if saveErr := s.saveSmokeAttempt(projectRef, sprintRef, SmokeResult{Project: projectRef, Sprint: sprintRef, Status: SmokeRunning}, nil, false); saveErr != nil {
		return SmokeResult{Project: projectRef, Sprint: sprintRef, Status: SmokeFailed}, smokeError("smoke_state_write", "persistence", "smoke attempt could not be recorded", "Repair flow-state persistence before retrying; no harness work was started.", saveErr)
	}
	result, err = s.runSmoke(ctx, projectRef, sprintRef, req)
	if saveErr := s.saveSmokeAttempt(projectRef, sprintRef, result, err, true); saveErr != nil {
		stateErr := smokeError("smoke_state_write", "persistence", "terminal smoke state could not be recorded", "Inspect and repair flow-state persistence before retrying.", saveErr)
		return result, errors.Join(err, stateErr)
	}
	return result, err
}

func (s Service) runSmoke(ctx context.Context, projectRef, sprintRef string, req SmokeRequest) (SmokeResult, error) {
	result := SmokeResult{Project: projectRef, Sprint: sprintRef, Status: SmokeReady, DryRun: req.DryRun, OverrideRationale: strings.TrimSpace(req.OverrideRationale)}
	emit := func(p SmokeProgress) {
		if req.Progress != nil {
			req.Progress(p)
		}
	}
	emit(SmokeProgress{Phase: SmokePhasePreflight, Message: "validating review gate and harness manifest"})
	prepared, err := s.prepareSmokeStatic(projectRef, sprintRef, req)
	if err != nil {
		return smokeFailedResult(result, err)
	}
	result.Project, result.Sprint = prepared.Sprint.Project, prepared.Sprint.Slug
	result.Harness, result.Protocol = prepared.Manifest.Harness.ID, prepared.Manifest.ProtocolVersion
	result.Artifact = ArtifactRelPath(prepared.Sprint, StageSmoke)
	result.ReviewVerdict, result.ReviewFingerprint = prepared.Review.Verdict, prepared.Review.Fingerprint
	result.ReviewOverride = req.ForceReview
	result.Ready = true
	result.EffectiveTimeout, result.TimeoutSource = smokeTimeout(s.smokeSettings, prepared.Manifest, req)
	if !req.DryRun {
		emit(SmokeProgress{Phase: SmokePhaseAuthoring, Message: "authoring sprint-specific deep-smoke coverage"})
		if err := s.authorSmokeSuite(ctx, prepared, &result); err != nil {
			return smokeFailedResult(result, err)
		}
	}

	getenv := s.smokeSettings.Getenv
	if getenv == nil {
		getenv = os.Getenv
	}
	env := smokeEnvironment(s.smokeSettings, prepared.Manifest, getenv)
	discoveryArgs := append(append([]string{}, prepared.Manifest.Args...), prepared.Manifest.Commands.Discover...)
	discoveryArgs = append(discoveryArgs, "--target", prepared.Target)
	emit(SmokeProgress{Phase: SmokePhaseDiscovery, Message: "discovering machine-readable smoke scopes"})
	discoveryProcess, discoveryErr := s.processRunner.Run(ctx, pprocess.Request{Executable: prepared.Executable, Args: discoveryArgs, Dir: prepared.CWD, Env: env, Timeout: s.smokeSettings.DiscoveryTimeout, StdoutLimit: s.smokeSettings.StdoutLimit, StderrLimit: s.smokeSettings.StderrLimit, CleanupGrace: s.smokeSettings.CleanupGrace})
	if discoveryErr != nil {
		return smokeFailedResult(result, classifyProcessSmokeError("discovery", discoveryProcess, discoveryErr))
	}
	if discoveryProcess.StdoutTruncated {
		return smokeFailedResult(result, smokeError("smoke_discovery_truncated", "protocol", "discovery output exceeded the configured bound", "Increase the bounded capture limit or reduce discovery output.", nil))
	}
	var discovery smokeDiscovery
	if err := decodeOneJSON(discoveryProcess.Stdout, &discovery); err != nil {
		return smokeFailedResult(result, smokeError("smoke_discovery_malformed", "protocol", "discovery did not return one valid JSON object", "Fix the harness discovery command.", err))
	}
	if err := validateSmokeDiscovery(discovery, prepared.Manifest); err != nil {
		return smokeFailedResult(result, err)
	}
	result.CoverageMapping = smokeCoverageMapping(discovery, prepared.Sprint.Slug)
	populateSmokeCoverageRequirements(result.CoverageMapping, filepath.Join(prepared.Sprint.Path, "requirements.md"))
	emit(SmokeProgress{Phase: SmokePhaseSelection, Message: "selecting narrowest sufficient scope"})
	selection, err := selectSmoke(discovery, prepared.Sprint.Slug, req)
	if err != nil {
		return smokeFailedResult(result, err)
	}
	result.ScopeKind, result.Scope, result.ScopeRationale = selection.Kind, strings.Join(selection.IDs, ","), selection.Rationale
	result.DurationClass, result.CostClass = selection.DurationClass, selection.CostClass
	if result.DurationClass == "" {
		result.DurationClass = prepared.Manifest.Defaults.DurationClass
	}
	if result.CostClass == "" {
		result.CostClass = prepared.Manifest.Defaults.CostClass
	}
	result.EvidenceRoots = []string{prepared.RunsRoot, prepared.IssuesRoot}
	result.Prerequisites = selection.Prerequisites
	result.DiagnosticOnly = selection.DiagnosticOnly || (req.ForceReview && (prepared.Review.Verdict == ReviewFail || prepared.Review.Verdict == ReviewVerdictBlocked))
	if selection.Verdict == SmokeBlockedVerdict || selection.Verdict == SmokeNotApplicable {
		result.Status, result.Verdict, result.NextAction = SmokeCompleted, selection.Verdict, selection.NextAction
		if req.DryRun {
			return result, nil
		}
		return s.commitSmoke(prepared, result)
	}
	if selection.DiagnosticOnly {
		result.Diagnostics = append(result.Diagnostics, "selected diagnostic scope does not replace required containing-suite evidence")
		result.NextAction = "Run the complete containing suite before treating smoke as current."
	}
	runArgs := append(append([]string{}, prepared.Manifest.Args...), prepared.Manifest.Commands.Run...)
	runArgs = append(runArgs, "--project", prepared.Sprint.Project, "--sprint", prepared.Sprint.Slug, "--workspace", s.root, "--target", prepared.Target, "--scope-kind", selection.Kind, "--scope", strings.Join(selection.IDs, ","))
	result.SafeArgv = safeArgv(prepared.Executable, runArgs)
	if req.DryRun {
		result.Status = SmokeReady
		result.NextAction = "Confirm and run the selected smoke scope."
		return result, nil
	}
	result.Status = SmokeRunning
	emit(SmokeProgress{Phase: SmokePhaseRunning, Message: "running external smoke harness"})
	runProcess, runErr := s.processRunner.Run(ctx, pprocess.Request{Executable: prepared.Executable, Args: runArgs, Dir: prepared.CWD, Env: env, Timeout: result.EffectiveTimeout, StdoutLimit: s.smokeSettings.StdoutLimit, StderrLimit: s.smokeSettings.StderrLimit, CleanupGrace: s.smokeSettings.CleanupGrace, Progress: func(event pprocess.Event) { emit(SmokeProgress{Phase: SmokePhaseRunning, Message: "harness progress"}) }})
	if runProcess.DroppedEvents > 0 {
		result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("%d progress events were dropped", runProcess.DroppedEvents))
	}
	if runProcess.StdoutTruncated {
		return smokeFailedResult(result, smokeError("smoke_run_truncated", "process", "run response exceeded the configured stdout bound", "Increase the bounded capture limit or reduce run output.", nil))
	}
	var response smokeRunResponse
	decodeErr := decodeOneJSON(runProcess.Stdout, &response)
	if decodeErr != nil {
		if runErr != nil {
			return smokeFailedResult(result, classifyProcessSmokeError("run", runProcess, runErr))
		}
		return smokeFailedResult(result, smokeError("smoke_run_malformed", "protocol", "run did not return one valid JSON object", "Inspect external harness evidence and fix its protocol output.", decodeErr))
	}
	if runProcess.TimedOut || runProcess.Cancelled || !runProcess.CleanupComplete {
		return smokeFailedResult(result, classifyProcessSmokeError("run", runProcess, runErr))
	}
	emit(SmokeProgress{Phase: SmokePhaseValidatingEvidence, Message: "validating external evidence identity"})
	validated, issues, tests, err := validateSmokeRun(prepared, discovery, selection, response, runProcess)
	if err != nil {
		return smokeFailedResult(result, err)
	}
	result.RunID, result.Runtime, result.Model = response.RunID, response.Runtime, response.Model
	result.Duration = time.Duration(response.DurationMs) * time.Millisecond
	result.Counts = SmokeCounts{Total: response.Counts.Total, Passed: response.Counts.Passed, Failed: response.Counts.Failed, Skipped: response.Counts.Skipped, Errors: response.Counts.Errors}
	result.Evidence, result.Issues, result.Tests = validated, issues, tests
	result.Verdict = synthesizeSmokeVerdict(result.Counts, issues)
	result.Status = SmokeCompleted
	result.NextAction = nextSmokeAction(result)
	if result.DiagnosticOnly {
		return result, nil
	}
	emit(SmokeProgress{Phase: SmokePhaseWritingArtifact, Message: "writing validated smoke summary"})
	committed, commitErr := s.commitSmoke(prepared, result)
	if commitErr == nil {
		emit(SmokeProgress{Phase: SmokePhaseCompleted, Message: "smoke complete"})
	}
	return committed, commitErr
}

func (s Service) saveSmokeAttempt(projectRef, sprintRef string, result SmokeResult, runErr error, terminal bool) error {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return err
	}
	state, err := LoadFlowState(s.root, sp)
	if err != nil {
		if !errors.Is(err, ErrFlowStateMissing) {
			return err
		}
		snap, readErr := s.store.ReadArtifacts(sp)
		if readErr != nil {
			return readErr
		}
		state = NewFlowState(sp, DeriveStages(sp, snap, nil), s.now())
	}
	now := s.now().UTC()
	current := state.Smoke
	if current == nil {
		current = &SmokeStageState{Path: ArtifactRelPath(sp, StageSmoke)}
	}
	if !terminal {
		current.Status = SmokeRunning
		current.ActiveAttempt = &VerificationAttempt{ID: fmt.Sprintf("smoke-%d", now.UnixNano()), Status: AttemptRunning, StartedAt: now, HeartbeatAt: now, OwnerPID: os.Getpid()}
	} else {
		attempt := VerificationAttempt{ID: fmt.Sprintf("smoke-%d", now.UnixNano()), StartedAt: now}
		if current.ActiveAttempt != nil {
			attempt = *current.ActiveAttempt
		}
		attempt.CompletedAt = &now
		attempt.Status = smokeAttemptStatus(result.Status, runErr)
		if se, ok := AsSmokeError(runErr); ok {
			attempt.Category, attempt.Diagnostics, attempt.NextAction = se.Category, []string{safeError(se)}, se.Guidance
		}
		current.ActiveAttempt, current.LastAttempt, current.Status = nil, &attempt, result.Status
		if current.LastComplete != nil && result.Status != SmokeCompleted {
			current.Verdict, current.RunID, current.EvidenceID = current.LastComplete.Verdict, current.LastComplete.RunID, current.LastComplete.EvidenceID
		}
	}
	state.Smoke = current
	return SaveFlowState(s.root, sp, state)
}

func smokeAttemptStatus(status SmokeExecutionStatus, err error) AttemptStatus {
	if errors.Is(err, context.Canceled) || status == SmokeCancelled {
		return AttemptCancelled
	}
	if se, ok := AsSmokeError(err); ok {
		if se.Category == "timeout" {
			return AttemptTimedOut
		}
		if se.Category == "review_gate" || se.Category == "catalog" {
			return AttemptBlocked
		}
	}
	if status == SmokeCompleted {
		return AttemptCompleted
	}
	return AttemptFailed
}

func (s Service) SmokeStatus(projectRef, sprintRef string) (SmokeResult, error) {
	result := SmokeResult{Project: projectRef, Sprint: sprintRef, Artifact: "smoke.md"}
	prepared, err := s.prepareSmokeStatic(projectRef, sprintRef, SmokeRequest{})
	if err != nil {
		result.Diagnostics = []string{safeError(err)}
		return result, err
	}
	result.Project, result.Sprint, result.Harness, result.Protocol = prepared.Sprint.Project, prepared.Sprint.Slug, prepared.Manifest.Harness.ID, prepared.Manifest.ProtocolVersion
	result.Artifact, result.Ready = ArtifactRelPath(prepared.Sprint, StageSmoke), true
	result.ReviewVerdict, result.ReviewFingerprint = prepared.Review.Verdict, prepared.Review.Fingerprint
	state, err := LoadFlowState(s.root, prepared.Sprint)
	if err == nil && state.Smoke != nil {
		result.Status, result.Verdict, result.RunID, result.Stale, result.Reconciliation = state.Smoke.Status, state.Smoke.Verdict, state.Smoke.RunID, state.Smoke.Stale, state.Smoke.Reconciliation
		result.Diagnostics = append(result.Diagnostics, state.Smoke.Diagnostics...)
	}
	if result.Status == "" {
		result.Status = SmokeReady
		result.NextAction = "Run smoke preview to discover the effective scope."
	}
	return result, nil
}

func (s Service) ValidateSmoke(projectRef, sprintRef string) (ValidationResult, error) {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	path, err := ArtifactPath(s.root, sp, StageSmoke)
	if err != nil {
		return ValidationResult{}, err
	}
	data, readErr := os.ReadFile(path)
	var findings []ValidationFinding
	if readErr != nil {
		findings = append(findings, finding("smoke.md", "", ArtifactRelPath(sp, StageSmoke), "missing smoke summary", safeError(readErr), "Run sprint smoke after a current review."))
	} else {
		findings = append(findings, ValidateSmokeContent(string(data))...)
	}
	state, stateErr := LoadFlowState(s.root, sp)
	if stateErr != nil {
		findings = append(findings, finding("Smoke state", "", FlowStateRelPath(sp), "flow state unavailable", safeError(stateErr), "Restore valid flow state."))
	} else if state.Smoke == nil {
		findings = append(findings, finding("Smoke state", "", FlowStateRelPath(sp), "smoke state missing", "smoke.md and flow state are not reconciled", "Rerun smoke after reviewing the committed summary."))
	} else if data != nil {
		fingerprint := hashBytes(data)
		if state.Smoke.SmokeFingerprint != fingerprint {
			findings = append(findings, finding("Smoke state", "", FlowStateRelPath(sp), "artifact/state mismatch", "smoke fingerprint differs from flow state", "Reconcile by rerunning smoke; automated recovery is deferred."))
		}
	}
	return ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: ArtifactRelPath(sp, StageSmoke), Findings: findings}, nil
}

func validateSmokeRun(p smokePrepared, discovery smokeDiscovery, selection smokeSelection, response smokeRunResponse, proc pprocess.Result) ([]SmokeEvidence, []SmokeIssue, []SmokeTestResult, error) {
	if response.SchemaVersion != 1 || response.ProtocolVersion != p.Manifest.ProtocolVersion || response.HarnessID != p.Manifest.Harness.ID || response.RunID == "" {
		return nil, nil, nil, smokeError("smoke_evidence_identity", "evidence", "run identity does not match manifest/discovery", "Inspect the external run response.", nil)
	}
	if response.ScopeKind != selection.Kind || strings.Join(response.Scope, ",") != strings.Join(selection.IDs, ",") {
		return nil, nil, nil, smokeError("smoke_evidence_scope", "evidence", "reported scope does not match the request", "Fix the harness scope identity.", nil)
	}
	c := response.Counts
	if c.Total < 0 || c.Passed < 0 || c.Failed < 0 || c.Skipped < 0 || c.Errors < 0 || c.Passed+c.Failed+c.Skipped+c.Errors != c.Total {
		return nil, nil, nil, smokeError("smoke_evidence_counts", "evidence", "result counts are inconsistent", "Fix the harness count summary.", nil)
	}
	if response.DurationMs < 0 || len(response.Evidence) == 0 {
		return nil, nil, nil, smokeError("smoke_evidence_missing", "evidence", "duration or evidence identity is missing", "Emit complete protocol-v1 evidence.", nil)
	}
	expected := expectedSmokeTests(discovery, selection)
	actual := make([]string, 0, len(response.Tests))
	statusCounts := SmokeCountsWire{}
	for _, test := range response.Tests {
		if test.ID == "" {
			return nil, nil, nil, smokeError("smoke_evidence_tests", "evidence", "run contains an empty test identity", "Return every executed discovered test identity.", nil)
		}
		actual = append(actual, test.ID)
		switch test.Status {
		case "passed":
			statusCounts.Passed++
		case "failed":
			statusCounts.Failed++
		case "skipped":
			statusCounts.Skipped++
		case "error":
			statusCounts.Errors++
		default:
			return nil, nil, nil, smokeError("smoke_evidence_tests", "evidence", "run contains an unsupported test status", "Use passed, failed, skipped, or error.", nil)
		}
	}
	statusCounts.Total = len(response.Tests)
	sort.Strings(expected)
	sort.Strings(actual)
	if strings.Join(expected, "\x00") != strings.Join(actual, "\x00") || statusCounts != c {
		return nil, nil, nil, smokeError("smoke_evidence_tests", "evidence", "executed test identities/statuses do not match discovery and counts", "Run exactly the selected discovered tests and return their identities.", nil)
	}
	var evidence []SmokeEvidence
	for _, item := range response.Evidence {
		root := p.RunsRoot
		if item.Kind == "issue" {
			root = p.IssuesRoot
		}
		full := item.Path
		if !filepath.IsAbs(full) {
			full = filepath.Join(p.HarnessRoot, filepath.FromSlash(full))
		}
		resolved, err := canonicalFileInside(root, full)
		if err != nil {
			return nil, nil, nil, smokeError("smoke_evidence_path", "evidence", "evidence path escapes or is missing", "Keep evidence inside declared roots.", err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, nil, nil, err
		}
		hash, err := hashFile(resolved)
		if err != nil {
			return nil, nil, nil, err
		}
		if item.SHA256 != "" && !strings.EqualFold(item.SHA256, hash) {
			return nil, nil, nil, smokeError("smoke_evidence_hash", "evidence", "evidence hash mismatch", "Restore immutable run evidence or rerun smoke.", nil)
		}
		if !strings.Contains(filepath.Base(resolved), response.RunID) && item.Kind != "issue" {
			return nil, nil, nil, smokeError("smoke_evidence_run", "evidence", "evidence path does not identify the reported run", "Use run-ID-addressed evidence.", nil)
		}
		evidence = append(evidence, SmokeEvidence{Kind: item.Kind, Path: resolved, SHA256: hash, Size: info.Size(), ModifiedAt: info.ModTime().UTC()})
	}
	issues := append([]SmokeIssue(nil), response.Issues...)
	failedTests := make(map[string]struct{}, c.Failed+c.Errors)
	for _, test := range response.Tests {
		if test.Status == "failed" || test.Status == "error" {
			failedTests[test.ID] = struct{}{}
		}
	}
	filedTests := make(map[string]struct{}, len(issues))
	for i := range issues {
		if issues[i].ID == "" || (issues[i].Status != "open" && issues[i].Status != "resolved") {
			return nil, nil, nil, smokeError("smoke_issue_identity", "evidence", "issue identity/status is invalid", "Emit open or resolved issue metadata.", nil)
		}
		full := issues[i].Path
		if !filepath.IsAbs(full) {
			full = filepath.Join(p.HarnessRoot, filepath.FromSlash(full))
		}
		resolved, err := canonicalFileInside(p.IssuesRoot, full)
		if err != nil || !strings.Contains(filepath.Base(resolved), issues[i].ID) {
			return nil, nil, nil, smokeError("smoke_issue_path", "evidence", "issue path is missing, escaping, or mismatched", "Use an issue-ID-addressed file under the issues root.", err)
		}
		issues[i].Path = resolved
		if _, failed := failedTests[issues[i].TestID]; failed && issues[i].Status == "open" {
			filedTests[issues[i].TestID] = struct{}{}
		}
		if issues[i].Status == "open" && (issues[i].TestID == "" || issues[i].Severity == "" || issues[i].Title == "" || issues[i].Summary == "" || issues[i].Theory == "" || issues[i].Evidence == "" || issues[i].Action == "") {
			return nil, nil, nil, smokeError("smoke_issue_detail", "evidence", "open issue metadata is incomplete", "For every open issue emit test_id, severity, title, summary, theory, evidence, and action.", nil)
		}
	}
	for testID := range failedTests {
		if _, ok := filedTests[testID]; !ok {
			return nil, nil, nil, smokeError("smoke_issue_missing", "evidence", "failed test has no detailed open issue: "+testID, "File and return one detailed issue for every failed or errored test.", nil)
		}
	}
	if proc.ExitCode != 0 && c.Failed+c.Errors == 0 {
		return nil, nil, nil, smokeError("smoke_process_unexplained", "process", "non-zero process exit has no matching failed evidence", "Inspect external evidence before retrying.", nil)
	}
	return evidence, issues, append([]SmokeTestResult(nil), response.Tests...), nil
}

func expectedSmokeTests(discovery smokeDiscovery, selection smokeSelection) []string {
	testBySuite := map[string][]string{}
	for _, suite := range discovery.Suites {
		testBySuite[suite.ID] = append([]string(nil), suite.Tests...)
	}
	var out []string
	switch selection.Kind {
	case "test":
		out = append(out, selection.IDs...)
	case "suite":
		for _, suite := range selection.IDs {
			out = append(out, testBySuite[suite]...)
		}
	case "level":
		for _, level := range discovery.Levels {
			if len(selection.IDs) > 0 && level.ID == selection.IDs[0] {
				for _, suite := range level.Suites {
					out = append(out, testBySuite[suite]...)
				}
			}
		}
	}
	return out
}

func (s Service) commitSmoke(p smokePrepared, result SmokeResult) (SmokeResult, error) {
	content := RenderSmoke(result)
	if findings := ValidateSmokeContent(content); len(findings) > 0 {
		return smokeFailedResult(result, smokeError("smoke_artifact_validation", "validation", findings[0].Problem, findings[0].Suggestion, nil))
	}
	path, err := ArtifactPath(s.root, p.Sprint, StageSmoke)
	if err != nil {
		return smokeFailedResult(result, err)
	}
	if err := atomicWriteFile(path, []byte(content)); err != nil {
		return smokeFailedResult(result, smokeError("smoke_persistence", "persistence", "smoke summary could not be committed", "The previous valid summary was preserved.", err))
	}
	state, err := LoadFlowState(s.root, p.Sprint)
	if err != nil {
		result.Reconciliation = true
		return result, smokeError("smoke_reconciliation", "reconciliation", "smoke.md committed but flow state could not be loaded", "Reconcile flow state by rerunning smoke.", err)
	}
	now := s.now().UTC()
	evidenceID := ""
	if len(result.Evidence) > 0 {
		evidenceID = result.Evidence[0].SHA256
	}
	artifactDigest := hashBytes([]byte(content))
	identityRefs := smokeIdentityReferences(p, result)
	inputFingerprint, _ := refreshEvidenceFingerprint(identityRefs)
	refs := make([]EvidenceReference, 0, len(result.Evidence))
	for _, evidence := range result.Evidence {
		refs = append(refs, EvidenceReference{Kind: evidence.Kind, Path: evidence.Path, Digest: evidence.SHA256})
	}
	var override *DiagnosticOverride
	if result.ReviewOverride {
		override = &DiagnosticOverride{Requested: true, Confirmed: true, Rationale: result.OverrideRationale, RequestedAt: now, ReviewFingerprint: result.ReviewFingerprint, ReviewVerdict: string(result.ReviewVerdict)}
	}
	active := (*VerificationAttempt)(nil)
	if state.Smoke != nil {
		active = state.Smoke.ActiveAttempt
	}
	state.Smoke = &SmokeStageState{Status: result.Status, Verdict: result.Verdict, Path: ArtifactRelPath(p.Sprint, StageSmoke), LastRunAt: &now, ReviewFingerprint: result.ReviewFingerprint, SmokeFingerprint: artifactDigest, ArtifactDigest: artifactDigest, InputFingerprint: inputFingerprint, RunID: result.RunID, AuthorRunID: result.AuthorRunID, AuthorModel: result.AuthorModel, AuthorChangedPaths: append([]string(nil), result.AuthorChangedPaths...), CoverageMapping: result.CoverageMapping, EvidenceID: evidenceID, ReviewOverride: result.ReviewOverride, Diagnostics: result.Diagnostics, Issues: append([]SmokeIssue(nil), result.Issues...), Evidence: refs, Override: override, ActiveAttempt: active, LastComplete: &SmokeCompletion{Verdict: result.Verdict, Artifact: ArtifactRelPath(p.Sprint, StageSmoke), ArtifactDigest: artifactDigest, InputFingerprint: inputFingerprint, CompletedAt: now, RunID: result.RunID, AuthorRunID: result.AuthorRunID, AuthorModel: result.AuthorModel, AuthorChangedPaths: append([]string(nil), result.AuthorChangedPaths...), CoverageMapping: result.CoverageMapping, EvidenceID: evidenceID, Evidence: identityRefs, Issues: append([]SmokeIssue(nil), result.Issues...), Override: override}}
	if err := SaveFlowState(s.root, p.Sprint, state); err != nil {
		result.Reconciliation = true
		return result, smokeError("smoke_reconciliation", "reconciliation", "smoke.md committed but flow state update failed", "Reconcile flow state by rerunning smoke.", err)
	}
	return result, nil
}

func RenderSmoke(r SmokeResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Sprint Smoke\n\nSmoke status: `%s`\nVerdict: `%s`\nDate: `%s`\n\n", r.Status, r.Verdict, time.Now().UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "## Smoke Context\n\nProject: `%s`\nSprint: `%s`\nArtifact: `%s`\n\n", r.Project, r.Sprint, r.Artifact)
	fmt.Fprintf(&b, "## Review Gate\n\nReview verdict: `%s`\nReview fingerprint: `%s`\nDiagnostic override: `%t`\nOverride rationale: %s\n\n", r.ReviewVerdict, r.ReviewFingerprint, r.ReviewOverride, printable(r.OverrideRationale))
	fmt.Fprintf(&b, "## Harness And Protocol\n\nHarness: `%s`\nProtocol: `%s`\n\n", r.Harness, r.Protocol)
	fmt.Fprintf(&b, "## Smoke Authoring\n\nAuthor run ID: `%s`\nAuthor model: `%s`\nChanged harness paths:\n", printable(r.AuthorRunID), printable(r.AuthorModel))
	if len(r.AuthorChangedPaths) == 0 {
		b.WriteString("- none; existing traceable suite retained after agent inspection\n")
	} else {
		for _, path := range r.AuthorChangedPaths {
			fmt.Fprintf(&b, "- `%s`\n", path)
		}
	}
	b.WriteString("\n")
	if mapping := r.CoverageMapping; mapping != nil {
		fmt.Fprintf(&b, "## Coverage Mapping\n\nComplete: `%t`\nRequired coverage: `%s`\nRationale: %s\n\n", mapping.Complete, strings.Join(mapping.RequiredCoverage, "`, `"), printable(mapping.Rationale))
		for _, requirement := range mapping.Requirements {
			fmt.Fprintf(&b, "- `%s` — %s (mapped tests: %s)\n", requirement.ID, printable(requirement.Description), printable(strings.Join(requirement.MappedTests, ", ")))
		}
		b.WriteString("\nTests:\n")
		for _, test := range mapping.Tests {
			fmt.Fprintf(&b, "- `%s` (suite `%s`): `%s`\n", test.ID, test.Suite, strings.Join(test.Coverage, "`, `"))
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "## Selected Scope And Rationale\n\nScope kind: `%s`\nScope: `%s`\nRationale: %s\nDuration class: `%s`\nCost class: `%s`\nDiagnostic only: `%t`\n\n", printable(r.ScopeKind), printable(r.Scope), printable(r.ScopeRationale), printable(r.DurationClass), printable(r.CostClass), r.DiagnosticOnly)
	fmt.Fprintf(&b, "## Preconditions And Environment\n\nPrerequisites: %s\nEnvironment: bounded allowlist; values not persisted\nEvidence roots: `%s`\nEffective timeout: `%s` (source `%s`)\n\n", printable(strings.Join(r.Prerequisites, "; ")), printable(strings.Join(r.EvidenceRoots, ", ")), r.EffectiveTimeout, printable(r.TimeoutSource))
	fmt.Fprintf(&b, "## Safe Invocation\n\nArgv: `%s`\n\n", printable(r.SafeArgv))
	fmt.Fprintf(&b, "## Run Evidence\n\nRun ID: `%s`\nTotal: `%d`\nPassed: `%d`\nFailed: `%d`\nSkipped: `%d`\nErrors: `%d`\nDuration: `%s`\nRuntime: `%s`\nModel: `%s`\n\n", printable(r.RunID), r.Counts.Total, r.Counts.Passed, r.Counts.Failed, r.Counts.Skipped, r.Counts.Errors, r.Duration, printable(r.Runtime), printable(r.Model))
	if len(r.Tests) > 0 {
		b.WriteString("Executed tests:\n")
		for _, test := range r.Tests {
			fmt.Fprintf(&b, "- `%s`: `%s`\n", test.ID, test.Status)
		}
		b.WriteString("\n")
	}
	b.WriteString("### External Evidence Identity And Links\n\n")
	if len(r.Evidence) == 0 {
		b.WriteString("- none (preflight classification)\n")
	} else {
		for _, e := range r.Evidence {
			fmt.Fprintf(&b, "- `%s` `%s` sha256 `%s` size `%d` modified `%s`\n", e.Kind, e.Path, e.SHA256, e.Size, e.ModifiedAt.Format(time.RFC3339))
		}
	}
	b.WriteString("\n## Findings\n\n")
	findings := 0
	for _, issue := range r.Issues {
		if issue.Status != "open" {
			continue
		}
		findings++
		fmt.Fprintf(&b, "### `%s` — %s\n\n", issue.TestID, printable(issue.Title))
		fmt.Fprintf(&b, "- Severity: `%s`\n", printable(issue.Severity))
		fmt.Fprintf(&b, "- Observed: %s\n", printable(issue.Summary))
		fmt.Fprintf(&b, "- Working theory: %s\n", printable(issue.Theory))
		fmt.Fprintf(&b, "- Supporting evidence: %s\n", printable(issue.Evidence))
		fmt.Fprintf(&b, "- Next investigation: %s\n\n", printable(issue.Action))
	}
	if len(r.Diagnostics) == 0 && findings == 0 {
		b.WriteString("- none\n")
	} else {
		for _, diagnostic := range r.Diagnostics {
			fmt.Fprintf(&b, "- %s\n", printable(diagnostic))
		}
	}
	b.WriteString("\n## Open Issues\n\n")
	open := false
	for _, issue := range r.Issues {
		if issue.Status == "open" {
			open = true
			fmt.Fprintf(&b, "- `%s` (%s, test `%s`): `%s`\n", issue.ID, printable(issue.Severity), printable(issue.TestID), issue.Path)
		}
	}
	if !open {
		b.WriteString("- none\n")
	}
	b.WriteString("\n## Resolved Issues\n\n")
	resolved := false
	for _, issue := range r.Issues {
		if issue.Status == "resolved" {
			resolved = true
			fmt.Fprintf(&b, "- `%s` `%s`\n", issue.ID, issue.Path)
		}
	}
	if !resolved {
		b.WriteString("- none\n")
	}
	b.WriteString("\n## Mutation And Safety Check\n\nOnly smoke.md, flow-state.json, manifest-declared harness authoring paths, and manifest-declared external evidence roots were approved for mutation. Product source and governed sprint inputs were identity-checked before and after authoring.\n\n")
	fmt.Fprintf(&b, "## Verdict And Next Action\n\nVerdict: `%s`\nNext action: %s\n", r.Verdict, printable(r.NextAction))
	return b.String()
}

func ValidateSmokeContent(content string) []ValidationFinding {
	var findings []ValidationFinding
	if strings.TrimSpace(content) == "" || containsPlaceholder(content) {
		findings = append(findings, finding("smoke.md", "", "", "empty or placeholder smoke summary", "summary is incomplete", "Generate a complete evidence-backed summary."))
	}
	for _, heading := range []string{"Smoke Context", "Review Gate", "Harness And Protocol", "Smoke Authoring", "Selected Scope And Rationale", "Preconditions And Environment", "Safe Invocation", "Run Evidence", "Findings", "Open Issues", "Resolved Issues", "Mutation And Safety Check", "Verdict And Next Action"} {
		if !markdownHeadingPresent(content, heading) {
			findings = append(findings, finding("smoke.md", heading, "", "missing required section", "section was not found", "Regenerate smoke.md."))
		}
	}
	verdict := SmokeVerdict(fieldBacktick(content, "Verdict:"))
	if !validSmokeVerdict(verdict) {
		findings = append(findings, finding("smoke.md", "Verdict", "", "invalid verdict", string(verdict), "Use one of the five smoke verdicts."))
	}
	for _, forbidden := range []string{"-----BEGIN", "Authorization: Bearer", "\"stdout\":", "\"stderr\":"} {
		if strings.Contains(content, forbidden) {
			findings = append(findings, finding("smoke.md", "Safety", "", "raw or secret-bearing content detected", forbidden, "Keep raw streams and secrets in external evidence only."))
		}
	}
	if !strings.Contains(content, "Next action:") {
		findings = append(findings, finding("smoke.md", "Next Action", "", "next action is missing", "no actionable terminal guidance", "Record the required next action."))
	}
	sortSprintFindings(findings)
	return findings
}

func classifyProcessSmokeError(stage string, result pprocess.Result, err error) error {
	switch {
	case result.TimedOut:
		return smokeError("smoke_timeout", "timeout", stage+" timed out", "Inspect external evidence and retry with a bounded timeout.", err)
	case result.Cancelled || errors.Is(err, context.Canceled):
		return smokeError("smoke_cancelled", "cancellation", stage+" was cancelled", "Confirm cleanup before retrying.", err)
	case !result.CleanupComplete:
		return smokeError("smoke_cleanup", "cleanup", stage+" cleanup is uncertain", "Terminate owned descendants before retrying.", err)
	default:
		return smokeError("smoke_process", "process", stage+" process failed", "Inspect the safe diagnostic and external harness logs.", err)
	}
}
func smokeFailedResult(result SmokeResult, err error) (SmokeResult, error) {
	result.Status = SmokeFailed
	if errors.Is(err, context.Canceled) {
		result.Status = SmokeCancelled
	}
	if se, ok := AsSmokeError(err); ok {
		result.Diagnostics = append(result.Diagnostics, se.Code+": "+se.Message)
	}
	return result, err
}
func decodeOneJSON(value string, target any) error {
	dec := json.NewDecoder(strings.NewReader(strings.TrimSpace(value)))
	if err := dec.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return fmt.Errorf("multiple JSON values")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
func safeArgv(executable string, args []string) string {
	// Manifest argv has no sensitivity metadata, so stable output retains only
	// option names and argument shape. Raw values remain in external evidence.
	values := []string{filepath.Base(executable)}
	for _, value := range args {
		if strings.HasPrefix(value, "-") {
			if name, _, ok := strings.Cut(value, "="); ok {
				values = append(values, name+"=[REDACTED]")
			} else {
				values = append(values, value)
			}
			continue
		}
		values = append(values, "[ARG]")
	}
	for i, value := range values {
		values[i] = fmt.Sprintf("%q", value)
	}
	return strings.Join(values, " ")
}
func hasOpenSmokeIssues(issues []SmokeIssue) bool {
	for _, issue := range issues {
		if issue.Status == "open" {
			return true
		}
	}
	return false
}
func synthesizeSmokeVerdict(counts SmokeCounts, issues []SmokeIssue) SmokeVerdict {
	if counts.Failed > 0 || counts.Errors > 0 {
		return SmokeFailVerdict
	}
	if hasOpenSmokeIssues(issues) {
		return SmokePassWithOpenIssues
	}
	return SmokePass
}
func nextSmokeAction(r SmokeResult) string {
	switch r.Verdict {
	case SmokePass:
		return "Deep smoke is complete; proceed to the next roadmap stage."
	case SmokePassWithOpenIssues:
		return "Review the linked open issues before proceeding."
	case SmokeFailVerdict:
		return "Inspect linked evidence, fix the selected-smoke failures, and rerun the containing suite."
	case SmokeBlockedVerdict:
		return "Satisfy the blocked prerequisite or coverage requirement and rerun smoke."
	case SmokeNotApplicable:
		return "No smoke action is required for this sprint."
	}
	return "Inspect smoke diagnostics."
}
func fieldBacktick(content, prefix string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), prefix)), "`")
		}
	}
	return ""
}
func printable(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "none"
	}
	value = strings.ReplaceAll(value, "`", "'")
	return value
}
func hashBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}
func atomicWriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".smoke.*.tmp")
	if err != nil {
		return err
	}
	temp := file.Name()
	keep := true
	defer func() {
		if keep {
			_ = os.Remove(temp)
		}
	}()
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Rename(temp, path); err != nil {
		return err
	}
	keep = false
	syncDir(filepath.Dir(path))
	return nil
}
