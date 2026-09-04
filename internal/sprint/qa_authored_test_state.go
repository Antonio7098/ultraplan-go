package sprint

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (store QAStore) LoadTestBundle(attemptID, testID string, budgets QABudgets) (QAReproductionSpec, QATestBundle, error) {
	if !validQAIDKind(attemptID, "attempt") || !validQAV2ID(testID, "test") {
		return QAReproductionSpec{}, QATestBundle{}, NewQAError(QAErrorInvalidState, "load authored test", "invalid attempt or test identity", nil)
	}
	var spec QAReproductionSpec
	path, err := store.resolve(QAReproductionSpecRelPath(store.sprint, attemptID, testID))
	if err != nil {
		return QAReproductionSpec{}, QATestBundle{}, err
	}
	if err := store.readStrictVersion(path, "reproduction-spec", QAEvidenceSchemaVersion, &spec); err != nil {
		return QAReproductionSpec{}, QATestBundle{}, err
	}
	if err := ValidateQAReproductionSpec(spec, budgets); err != nil || spec.AttemptID != attemptID {
		return QAReproductionSpec{}, QATestBundle{}, NewQAError(QAErrorInvalidState, "load authored test", "invalid reproduction specification", err)
	}
	var bundle QATestBundle
	path, err = store.resolve(QATestBundleRelPath(store.sprint, attemptID, testID))
	if err != nil {
		return QAReproductionSpec{}, QATestBundle{}, err
	}
	if err := store.readStrictVersion(path, "test-bundle", QAEvidenceSchemaVersion, &bundle); err != nil {
		return QAReproductionSpec{}, QATestBundle{}, err
	}
	if bundle.ID != testID {
		return QAReproductionSpec{}, QATestBundle{}, NewQAError(QAErrorInvalidState, "load authored test", "test bundle identity does not match its path", nil)
	}
	if err := ValidateQATestBundle(bundle, spec, budgets); err != nil {
		return QAReproductionSpec{}, QATestBundle{}, NewQAError(QAErrorInvalidState, "load authored test", err.Error(), err)
	}
	for _, file := range bundle.Files {
		materialized, resolveErr := store.resolve(QATestFileRelPath(store.sprint, attemptID, testID, file.Path))
		if resolveErr != nil {
			return QAReproductionSpec{}, QATestBundle{}, resolveErr
		}
		data, readErr := os.ReadFile(materialized)
		if readErr != nil || string(data) != file.Content {
			return QAReproductionSpec{}, QATestBundle{}, NewQAError(QAErrorInvalidState, "load authored test", "materialized authored test does not match its immutable bundle", readErr)
		}
	}
	return spec, bundle, nil
}

func (store QAStore) ListTestBundles(attemptID string, budgets QABudgets) ([]QATestBundle, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return nil, NewQAError(QAErrorInvalidState, "list authored tests", "invalid attempt identity", nil)
	}
	root, err := store.resolve(filepath.ToSlash(filepath.Join("projects", store.sprint.Project, "sprints", store.sprint.Slug, "verification", "attempts", attemptID, "investigator-tests")))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return []QATestBundle{}, nil
	}
	if err != nil {
		return nil, NewQAError(QAErrorPersistenceFailure, "list authored tests", "cannot list authored tests", err)
	}
	values := make([]QATestBundle, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validQAV2ID(entry.Name(), "test") {
			return nil, NewQAError(QAErrorInvalidState, "list authored tests", "authored-test storage contains an invalid entry", nil)
		}
		_, bundle, loadErr := store.LoadTestBundle(attemptID, entry.Name(), budgets)
		if loadErr != nil {
			return nil, loadErr
		}
		values = append(values, bundle)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	return values, nil
}

func (store QAStore) LoadTestPublications(attemptID string, budgets QABudgets) ([]QATestPublication, error) {
	bundles, err := store.ListTestBundles(attemptID, budgets)
	if err != nil {
		return nil, err
	}
	publications := make([]QATestPublication, 0, len(bundles))
	for _, listed := range bundles {
		spec, bundle, loadErr := store.LoadTestBundle(attemptID, listed.ID, budgets)
		if loadErr != nil {
			return nil, loadErr
		}
		runs, loadErr := store.ListReproductionRuns(attemptID, bundle.ID, spec, bundle)
		if loadErr != nil {
			return nil, loadErr
		}
		authoring, loadErr := store.ListTestAuthoringAttempts(attemptID, bundle.ID)
		if loadErr != nil {
			return nil, loadErr
		}
		publications = append(publications, QATestPublication{Spec: spec, Bundle: bundle, AuthoringAttempts: authoring, Runs: runs})
	}
	return publications, nil
}

func (store QAStore) LoadReproductionRun(attemptID, testID, runID string, spec QAReproductionSpec, bundle QATestBundle) (QAReproductionRun, error) {
	if !validQAIDKind(attemptID, "attempt") || !validQAV2ID(testID, "test") || !validQAV2ID(runID, "run") {
		return QAReproductionRun{}, NewQAError(QAErrorInvalidState, "load reproduction run", "invalid run identity", nil)
	}
	path, err := store.resolve(QAReproductionRunResultRelPath(store.sprint, attemptID, testID, runID))
	if err != nil {
		return QAReproductionRun{}, err
	}
	var run QAReproductionRun
	if err := store.readStrictVersion(path, "reproduction-run", QAEvidenceSchemaVersion, &run); err != nil {
		return QAReproductionRun{}, err
	}
	if run.ID != runID {
		return QAReproductionRun{}, NewQAError(QAErrorInvalidState, "load reproduction run", "run identity does not match its path", nil)
	}
	if err := ValidateQAReproductionRun(run, spec, bundle); err != nil {
		return QAReproductionRun{}, NewQAError(QAErrorInvalidState, "load reproduction run", err.Error(), err)
	}
	return run, nil
}

func (store QAStore) ListReproductionRuns(attemptID, testID string, spec QAReproductionSpec, bundle QATestBundle) ([]QAReproductionRun, error) {
	root, err := store.resolve(filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(store.sprint, attemptID, testID), "runs")))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return []QAReproductionRun{}, nil
	}
	if err != nil {
		return nil, NewQAError(QAErrorPersistenceFailure, "list reproduction runs", "cannot list reproduction runs", err)
	}
	runs := make([]QAReproductionRun, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !validQAV2ID(entry.Name(), "run") {
			return nil, NewQAError(QAErrorInvalidState, "list reproduction runs", "reproduction run storage contains an invalid entry", nil)
		}
		run, loadErr := store.LoadReproductionRun(attemptID, testID, entry.Name(), spec, bundle)
		if loadErr != nil {
			return nil, loadErr
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CompletedAt.Equal(runs[j].CompletedAt) {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].CompletedAt.Before(runs[j].CompletedAt)
	})
	return runs, nil
}

func (store QAStore) ListTestAuthoringAttempts(attemptID, testID string) ([]QAInvestigatorAttempt, error) {
	if !validQAIDKind(attemptID, "attempt") || !validQAV2ID(testID, "test") {
		return nil, NewQAError(QAErrorInvalidState, "list test authoring attempts", "invalid attempt or test identity", nil)
	}
	root, err := store.resolve(filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(store.sprint, attemptID, testID), "authoring-attempts")))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return []QAInvestigatorAttempt{}, nil
	}
	if err != nil {
		return nil, NewQAError(QAErrorPersistenceFailure, "list test authoring attempts", "cannot list test authoring attempts", err)
	}
	attempts := make([]QAInvestigatorAttempt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil, NewQAError(QAErrorInvalidState, "list test authoring attempts", "authoring-attempt storage contains an invalid entry", nil)
		}
		var envelope struct {
			SchemaVersion int                   `json:"schema_version"`
			Attempt       QAInvestigatorAttempt `json:"attempt"`
		}
		path, resolveErr := store.resolve(filepath.ToSlash(filepath.Join(QAInvestigatorTestRelPath(store.sprint, attemptID, testID), "authoring-attempts", entry.Name())))
		if resolveErr != nil {
			return nil, resolveErr
		}
		if readErr := store.readStrictVersion(path, "test-authoring-attempt", QAEvidenceSchemaVersion, &envelope); readErr != nil {
			return nil, readErr
		}
		attempt := envelope.Attempt
		if attempt.Number <= 0 || entry.Name() != fmt.Sprintf("%06d.json", attempt.Number) || attempt.SessionID == "" || attempt.Provider == "" || attempt.Model == "" || attempt.RuntimeStoreRef == "" || attempt.WorkspaceID == "" {
			return nil, NewQAError(QAErrorInvalidState, "list test authoring attempts", "retained authoring identity is incomplete", nil)
		}
		attempts = append(attempts, attempt)
	}
	sort.Slice(attempts, func(i, j int) bool { return attempts[i].Number < attempts[j].Number })
	return attempts, nil
}

func (store QAStore) LoadInvestigatorWorkspaceCleanup(attemptID string) (QACleanupFacts, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return QACleanupFacts{}, NewQAError(QAErrorInvalidState, "load investigator workspace cleanup", "invalid attempt identity", nil)
	}
	path, err := store.resolve(QAInvestigatorWorkspaceCleanupRelPath(store.sprint, attemptID))
	if err != nil {
		return QACleanupFacts{}, err
	}
	var envelope struct {
		SchemaVersion int            `json:"schema_version"`
		AttemptID     string         `json:"attempt_id"`
		Cleanup       QACleanupFacts `json:"cleanup"`
	}
	if err := store.readStrictVersion(path, "investigator-workspace-cleanup", QAEvidenceSchemaVersion, &envelope); err != nil {
		return QACleanupFacts{}, err
	}
	if envelope.AttemptID != attemptID || !envelope.Cleanup.Attempted || envelope.Cleanup.Complete && (!envelope.Cleanup.DescendantsTerminated || !envelope.Cleanup.WorkspaceRemoved) {
		return QACleanupFacts{}, NewQAError(QAErrorInvalidState, "load investigator workspace cleanup", "cleanup facts are invalid", nil)
	}
	return envelope.Cleanup, nil
}

func (store QAStore) LoadArbiterEvidenceRequest(attemptID, requestID string) (QAArbiterEvidenceRequest, error) {
	if !validQAIDKind(attemptID, "attempt") || !validQAV2ID(requestID, "request") {
		return QAArbiterEvidenceRequest{}, NewQAError(QAErrorInvalidState, "load arbiter evidence request", "invalid request identity", nil)
	}
	path, err := store.resolve(QAArbiterEvidenceRequestRelPath(store.sprint, attemptID, requestID))
	if err != nil {
		return QAArbiterEvidenceRequest{}, err
	}
	var envelope struct {
		SchemaVersion int                      `json:"schema_version"`
		AttemptID     string                   `json:"attempt_id"`
		Request       QAArbiterEvidenceRequest `json:"request"`
	}
	if err := store.readStrictVersion(path, "arbiter-evidence-request", QAEvidenceSchemaVersion, &envelope); err != nil {
		return QAArbiterEvidenceRequest{}, err
	}
	if envelope.AttemptID != attemptID || envelope.Request.ID != requestID {
		return QAArbiterEvidenceRequest{}, NewQAError(QAErrorInvalidState, "load arbiter evidence request", "request identity does not match its path", nil)
	}
	return envelope.Request, nil
}

func (store QAStore) ListArbiterEvidenceRequests(attemptID string) ([]QAArbiterEvidenceRequest, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return nil, NewQAError(QAErrorInvalidState, "list arbiter evidence requests", "invalid attempt identity", nil)
	}
	root, err := store.resolve(filepath.ToSlash(filepath.Join("projects", store.sprint.Project, "sprints", store.sprint.Slug, "verification", "attempts", attemptID, "arbiter-evidence-requests")))
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return []QAArbiterEvidenceRequest{}, nil
	}
	if err != nil {
		return nil, NewQAError(QAErrorPersistenceFailure, "list arbiter evidence requests", "cannot list arbiter evidence requests", err)
	}
	requests := make([]QAArbiterEvidenceRequest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil, NewQAError(QAErrorInvalidState, "list arbiter evidence requests", "request storage contains an invalid entry", nil)
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		request, loadErr := store.LoadArbiterEvidenceRequest(attemptID, id)
		if loadErr != nil {
			return nil, loadErr
		}
		requests = append(requests, request)
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].ID < requests[j].ID })
	return requests, nil
}

func (store QAStore) LoadIssueEvidenceCoverage(attemptID string) ([]QAIssueEvidenceCoverage, error) {
	if !validQAIDKind(attemptID, "attempt") {
		return nil, NewQAError(QAErrorInvalidState, "load issue evidence coverage", "invalid attempt identity", nil)
	}
	path, err := store.resolve(QAIssueEvidenceCoverageRelPath(store.sprint, attemptID))
	if err != nil {
		return nil, err
	}
	var envelope struct {
		SchemaVersion int                       `json:"schema_version"`
		AttemptID     string                    `json:"attempt_id"`
		Issues        []QAIssueEvidenceCoverage `json:"issues"`
	}
	if err := store.readStrictVersion(path, "issue-evidence-coverage", QAEvidenceSchemaVersion, &envelope); err != nil {
		return nil, err
	}
	if envelope.AttemptID != attemptID {
		return nil, NewQAError(QAErrorInvalidState, "load issue evidence coverage", "coverage identity does not match its path", nil)
	}
	for _, value := range envelope.Issues {
		if err := ValidateQAIssueEvidenceCoverage(value); err != nil {
			return nil, NewQAError(QAErrorInvalidState, "load issue evidence coverage", err.Error(), err)
		}
	}
	return envelope.Issues, nil
}

func (store QAStore) PublishReproductionRun(attemptID string, spec QAReproductionSpec, bundle QATestBundle, run QAReproductionRun, token QAWriterToken) error {
	if err := store.checkWriter(token); err != nil {
		return err
	}
	if !validQAIDKind(attemptID, "attempt") || spec.AttemptID != attemptID || bundle.ID != run.TestBundleID {
		return NewQAError(QAErrorInvalidState, "publish reproduction run", "run ownership is invalid", nil)
	}
	if err := ValidateQAReproductionRun(run, spec, bundle); err != nil {
		return NewQAError(QAErrorMalformedEvidence, "publish reproduction run", err.Error(), err)
	}
	path, err := store.resolve(QAReproductionRunResultRelPath(store.sprint, attemptID, bundle.ID, run.ID))
	if err != nil {
		return err
	}
	if _, err := store.writeRecord("reproduction-run", path, &run, true); err != nil {
		return err
	}
	runRoot := QAReproductionRunRelPath(store.sprint, attemptID, bundle.ID, run.ID)
	for name, content := range map[string]string{"stdout.log": run.Result.Stdout, "stderr.log": run.Result.Stderr} {
		logPath, resolveErr := store.resolve(filepath.ToSlash(filepath.Join(runRoot, name)))
		if resolveErr != nil {
			return resolveErr
		}
		if _, writeErr := store.writeBytes("reproduction-"+strings.TrimSuffix(name, ".log"), logPath, []byte(content), true); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func (store QAStore) PublishInvestigatorWorkspaceCleanup(attemptID string, facts QACleanupFacts, token QAWriterToken) error {
	if err := store.checkWriter(token); err != nil {
		return err
	}
	if !validQAIDKind(attemptID, "attempt") || !facts.Attempted {
		return NewQAError(QAErrorInvalidState, "publish investigator workspace cleanup", "invalid cleanup ownership or facts", nil)
	}
	path, err := store.resolve(QAInvestigatorWorkspaceCleanupRelPath(store.sprint, attemptID))
	if err != nil {
		return err
	}
	envelope := struct {
		SchemaVersion int            `json:"schema_version"`
		AttemptID     string         `json:"attempt_id"`
		Cleanup       QACleanupFacts `json:"cleanup"`
	}{QAEvidenceSchemaVersion, attemptID, facts}
	_, err = store.writeRecord("investigator-workspace-cleanup", path, &envelope, false)
	return err
}

func (store QAStore) publishAuthoredTests(bundle *QAEvidencePublication, attemptID string, token QAWriterToken) error {
	seenTests, seenRequests := map[string]bool{}, map[string]bool{}
	for _, publication := range bundle.InvestigatorTests {
		if publication.Spec.AttemptID != attemptID {
			return NewQAError(QAErrorMalformedEvidence, "publish authored test", "reproduction specification belongs to another attempt", nil)
		}
		if err := ValidateQAReproductionSpec(publication.Spec, bundle.Budgets); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish authored test", err.Error(), err)
		}
		if err := ValidateQATestBundle(publication.Bundle, publication.Spec, bundle.Budgets); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish authored test", err.Error(), err)
		}
		if seenTests[publication.Bundle.ID] {
			return NewQAError(QAErrorMalformedEvidence, "publish authored test", "duplicate authored test bundle", nil)
		}
		seenTests[publication.Bundle.ID] = true
		if err := store.writeAuthoredTest(publication, attemptID, bundle.Budgets, token); err != nil {
			return err
		}
	}
	for _, request := range bundle.EvidenceRequests {
		if !validQAV2ID(request.ID, "request") || !validQAIDKind(request.OriginShardID, "shard") || len(request.TheoryIDs) == 0 || seenRequests[request.ID] {
			return NewQAError(QAErrorMalformedEvidence, "publish arbiter evidence request", "invalid or duplicate evidence request", nil)
		}
		seenRequests[request.ID] = true
		path, err := store.resolve(QAArbiterEvidenceRequestRelPath(store.sprint, attemptID, request.ID))
		if err != nil {
			return err
		}
		envelope := struct {
			SchemaVersion int                      `json:"schema_version"`
			AttemptID     string                   `json:"attempt_id"`
			Request       QAArbiterEvidenceRequest `json:"request"`
		}{QAEvidenceSchemaVersion, attemptID, request}
		if _, err := store.writeRecord("arbiter-evidence-request", path, &envelope, false); err != nil {
			return err
		}
	}
	if len(bundle.IssueCoverage) > 0 {
		for _, coverage := range bundle.IssueCoverage {
			if err := ValidateQAIssueEvidenceCoverage(coverage); err != nil {
				return NewQAError(QAErrorMalformedEvidence, "publish issue evidence coverage", err.Error(), err)
			}
		}
		path, err := store.resolve(QAIssueEvidenceCoverageRelPath(store.sprint, attemptID))
		if err != nil {
			return err
		}
		envelope := struct {
			SchemaVersion int                       `json:"schema_version"`
			AttemptID     string                    `json:"attempt_id"`
			Issues        []QAIssueEvidenceCoverage `json:"issues"`
		}{QAEvidenceSchemaVersion, attemptID, bundle.IssueCoverage}
		if _, err := store.writeRecord("issue-evidence-coverage", path, &envelope, true); err != nil {
			return err
		}
	}
	return nil
}

func (store QAStore) writeAuthoredTest(publication QATestPublication, attemptID string, budgets QABudgets, token QAWriterToken) error {
	testID := publication.Bundle.ID
	for kind, record := range map[string]any{"reproduction-spec": &publication.Spec, "test-bundle": &publication.Bundle} {
		if err := store.checkWriter(token); err != nil {
			return err
		}
		var rel string
		if kind == "reproduction-spec" {
			rel = QAReproductionSpecRelPath(store.sprint, attemptID, testID)
		} else {
			rel = QATestBundleRelPath(store.sprint, attemptID, testID)
		}
		path, err := store.resolve(rel)
		if err != nil {
			return err
		}
		if _, err := store.writeRecord(kind, path, record, true); err != nil {
			return err
		}
	}
	for _, file := range publication.Bundle.Files {
		path, err := store.resolve(QATestFileRelPath(store.sprint, attemptID, testID, file.Path))
		if err != nil {
			return err
		}
		if _, err := store.writeBytes("authored-test-file", path, []byte(file.Content), true); err != nil {
			return err
		}
	}
	if len(publication.AuthoringAttempts) > budgets.EvidenceRoundsPerShard {
		return NewQAError(QAErrorBudgetExhausted, "publish authored test", "authoring attempts exceed the evidence-round budget", nil)
	}
	for i, attempt := range publication.AuthoringAttempts {
		number := attempt.Number
		if number <= 0 {
			number = i + 1
		}
		path, err := store.resolve(QATestAuthoringAttemptRelPath(store.sprint, attemptID, testID, number))
		if err != nil {
			return err
		}
		envelope := struct {
			SchemaVersion int                   `json:"schema_version"`
			Attempt       QAInvestigatorAttempt `json:"attempt"`
		}{QAEvidenceSchemaVersion, attempt}
		if _, err := store.writeRecord("test-authoring-attempt", path, &envelope, true); err != nil {
			return err
		}
	}
	for _, run := range publication.Runs {
		if err := ValidateQAReproductionRun(run, publication.Spec, publication.Bundle); err != nil {
			return NewQAError(QAErrorMalformedEvidence, "publish reproduction run", err.Error(), err)
		}
		path, err := store.resolve(QAReproductionRunResultRelPath(store.sprint, attemptID, testID, run.ID))
		if err != nil {
			return err
		}
		if _, err := store.writeRecord("reproduction-run", path, &run, true); err != nil {
			return err
		}
		runRoot := QAReproductionRunRelPath(store.sprint, attemptID, testID, run.ID)
		for name, content := range map[string]string{"stdout.log": run.Result.Stdout, "stderr.log": run.Result.Stderr} {
			logPath, resolveErr := store.resolve(filepath.ToSlash(filepath.Join(runRoot, name)))
			if resolveErr != nil {
				return resolveErr
			}
			if _, writeErr := store.writeBytes("reproduction-"+strings.TrimSuffix(name, ".log"), logPath, []byte(content), true); writeErr != nil {
				return writeErr
			}
		}
		for name, record := range map[string]any{"failure-signature.json": &run.Signature, "cleanup.json": &run.Cleanup} {
			path, resolveErr := store.resolve(filepath.ToSlash(filepath.Join(runRoot, name)))
			if resolveErr != nil {
				return resolveErr
			}
			envelope := struct {
				SchemaVersion int `json:"schema_version"`
				Value         any `json:"value"`
			}{QAEvidenceSchemaVersion, record}
			if _, writeErr := store.writeRecord(strings.TrimSuffix(name, ".json"), path, &envelope, true); writeErr != nil {
				return writeErr
			}
		}
	}
	return nil
}

func validateRetainedRuntimeIdentity(original QAInvestigatorAttempt, requestProvider, requestModel, requestVariant, runtimeStoreRef, workspaceID, sessionID string) error {
	if original.SessionID == "" || original.SessionID != sessionID || original.Provider != requestProvider || original.Model != requestModel || original.Variant != requestVariant || original.RuntimeStoreRef != runtimeStoreRef || original.WorkspaceID != workspaceID {
		return fmt.Errorf("original investigator session identity is unavailable or changed")
	}
	return nil
}
