package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const qaFailureMatcherLimit = 512

func FreezeQAReproductionSpec(project, sprint string, spec QAReproductionSpec, budgets QABudgets, now time.Time) (QAReproductionSpec, error) {
	spec.SchemaVersion = QAEvidenceSchemaVersion
	spec.TheoryIDs = normalizeQAStrings(spec.TheoryIDs)
	spec.Preconditions = normalizeQAStrings(spec.Preconditions)
	spec.InconclusiveConditions = normalizeQAStrings(spec.InconclusiveConditions)
	spec.ApprovedTestPaths = normalizeQAStrings(spec.ApprovedTestPaths)
	spec.FrozenAt = now.UTC()
	id, err := NewQAV2ID("spec", project, sprint, spec.ShardID, struct {
		Attempt, Shard, Claim, Expected, Implementation string
		Theories, Preconditions, Inconclusive, Paths    []string
		Failure                                         QAFailureSignature
		Command                                         QACheckDescriptor
	}{spec.AttemptID, spec.ShardID, spec.Claim, spec.ExpectedBehavior, spec.ImplementationFingerprint, spec.TheoryIDs, spec.Preconditions, spec.InconclusiveConditions, spec.ApprovedTestPaths, spec.PredictedFailure, spec.Command})
	if err != nil {
		return QAReproductionSpec{}, err
	}
	spec.ID = id
	if err := ValidateQAReproductionSpec(spec, budgets); err != nil {
		return QAReproductionSpec{}, err
	}
	return spec, nil
}

func ValidateQAReproductionSpec(spec QAReproductionSpec, budgets QABudgets) error {
	if spec.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(spec.ID, "spec") || !validQAIDKind(spec.AttemptID, "attempt") || !validQAIDKind(spec.ShardID, "shard") {
		return fmt.Errorf("invalid QA reproduction specification identity")
	}
	if len(spec.TheoryIDs) == 0 || len(spec.TheoryIDs) > budgets.TheoriesPerShard || strings.TrimSpace(spec.Claim) == "" || strings.TrimSpace(spec.ExpectedBehavior) == "" || len(spec.InconclusiveConditions) == 0 || spec.FrozenAt.IsZero() {
		return fmt.Errorf("QA reproduction specification is incomplete")
	}
	if !validFingerprint(spec.ImplementationFingerprint) {
		return fmt.Errorf("QA reproduction specification implementation fingerprint is invalid")
	}
	if len(spec.ApprovedTestPaths) == 0 || len(spec.ApprovedTestPaths) > budgets.AuthoredTestFiles {
		return fmt.Errorf("QA reproduction specification test paths exceed bounds")
	}
	seen := map[string]bool{}
	for _, path := range spec.ApprovedTestPaths {
		if err := validateQAPath(path); err != nil {
			return err
		}
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
		if !strings.HasSuffix(clean, "_test.go") || seen[clean] {
			return fmt.Errorf("approved authored-test path must be a unique _test.go file: %q", path)
		}
		seen[clean] = true
	}
	if err := ValidateQAFailureSignature(spec.PredictedFailure); err != nil {
		return err
	}
	if strings.TrimSpace(spec.Command.ID) == "" || strings.TrimSpace(spec.Command.Executable) == "" || spec.Command.Timeout <= 0 || spec.Command.Timeout > budgets.CommandTimeout || spec.Command.OutputLimit <= 0 || spec.Command.OutputLimit > budgets.CommandOutputBytes {
		return fmt.Errorf("QA reproduction command is invalid")
	}
	switch strings.ToLower(filepath.Base(spec.Command.Executable)) {
	case "sh", "bash", "zsh", "fish", "cmd", "powershell", "pwsh", "git":
		return fmt.Errorf("QA reproduction command executable is prohibited")
	}
	if spec.Command.WorkingDirectory != "" {
		if err := validateQAPath(spec.Command.WorkingDirectory); err != nil {
			return fmt.Errorf("QA reproduction working directory is invalid: %w", err)
		}
	}
	return nil
}

func ValidateQAFailureSignature(signature QAFailureSignature) error {
	signature.TestName = strings.TrimSpace(signature.TestName)
	signature.ExitClass = strings.TrimSpace(signature.ExitClass)
	signature.OutputMatcher = strings.TrimSpace(signature.OutputMatcher)
	if signature.TestName == "" || signature.ExitClass == "" || signature.OutputMatcher == "" || len(signature.OutputMatcher) > qaFailureMatcherLimit || strings.ContainsAny(signature.OutputMatcher, "\x00\r\n") {
		return fmt.Errorf("QA failure signature requires a test name, exit class, and bounded single-line output matcher")
	}
	if strings.TrimSpace(signature.Assertion) == "" && strings.TrimSpace(signature.ErrorCode) == "" {
		return fmt.Errorf("QA failure signature requires an assertion or structured error code")
	}
	if signature.ExitClass != "zero" && signature.ExitClass != "nonzero" && !strings.HasPrefix(signature.ExitClass, "exit:") {
		return fmt.Errorf("QA failure signature exit class is invalid")
	}
	if strings.HasPrefix(signature.ExitClass, "exit:") {
		if _, err := strconv.Atoi(strings.TrimPrefix(signature.ExitClass, "exit:")); err != nil {
			return fmt.Errorf("QA failure signature exit class is invalid")
		}
	}
	return nil
}

func BuildQATestBundle(project, sprint string, spec QAReproductionSpec, files []QATestFile, derivedFrom string, budgets QABudgets) (QATestBundle, error) {
	if err := ValidateQAReproductionSpec(spec, budgets); err != nil {
		return QATestBundle{}, err
	}
	if len(files) == 0 || len(files) > budgets.AuthoredTestFiles {
		return QATestBundle{}, fmt.Errorf("authored test bundle file count exceeds bounds")
	}
	allowed := make(map[string]bool, len(spec.ApprovedTestPaths))
	for _, path := range spec.ApprovedTestPaths {
		allowed[filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))] = true
	}
	seen, total := map[string]bool{}, 0
	for i := range files {
		files[i].Path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(files[i].Path)))
		if !allowed[files[i].Path] || seen[files[i].Path] || strings.ContainsRune(files[i].Content, 0) {
			return QATestBundle{}, fmt.Errorf("authored test file is outside the frozen allowlist: %q", files[i].Path)
		}
		seen[files[i].Path] = true
		files[i].Bytes = len([]byte(files[i].Content))
		digest := sha256.Sum256([]byte(files[i].Content))
		files[i].Digest = hex.EncodeToString(digest[:])
		total += files[i].Bytes
	}
	if total > budgets.AuthoredTestBytes {
		return QATestBundle{}, fmt.Errorf("authored test bundle byte count exceeds bounds")
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	contentDigest, err := fingerprintQAValue(files)
	if err != nil {
		return QATestBundle{}, err
	}
	bundle := QATestBundle{SchemaVersion: QAEvidenceSchemaVersion, SpecID: spec.ID, Files: files, ContentDigest: contentDigest, DerivedFrom: derivedFrom}
	bundle.ID, err = NewQAV2ID("test", project, sprint, spec.ID, struct {
		Digest, DerivedFrom string
	}{contentDigest, derivedFrom})
	if err != nil {
		return QATestBundle{}, err
	}
	if err := ValidateQATestBundle(bundle, spec, budgets); err != nil {
		return QATestBundle{}, err
	}
	return bundle, nil
}

// DeriveQATestBundle preserves the original immutable bundle and makes an
// explicit lineage edge for a convention-driven test edit. Callers must rerun
// the complete fail-before/pass-after proof for the returned bundle.
func DeriveQATestBundle(project, sprint string, spec QAReproductionSpec, original QATestBundle, files []QATestFile, budgets QABudgets) (QATestBundle, error) {
	if err := ValidateQATestBundle(original, spec, budgets); err != nil {
		return QATestBundle{}, fmt.Errorf("original authored test bundle is invalid: %w", err)
	}
	derived, err := BuildQATestBundle(project, sprint, spec, files, original.ID, budgets)
	if err != nil {
		return QATestBundle{}, err
	}
	if derived.ContentDigest == original.ContentDigest {
		return QATestBundle{}, fmt.Errorf("derived authored test bundle must change content")
	}
	return derived, nil
}

func ValidateQATestBundle(bundle QATestBundle, spec QAReproductionSpec, budgets QABudgets) error {
	if bundle.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(bundle.ID, "test") || bundle.SpecID != spec.ID || !validFingerprint(bundle.ContentDigest) {
		return fmt.Errorf("invalid authored test bundle identity")
	}
	if bundle.DerivedFrom != "" && !validQAV2ID(bundle.DerivedFrom, "test") {
		return fmt.Errorf("authored test bundle has invalid derivation")
	}
	if len(bundle.Files) == 0 || len(bundle.Files) > budgets.AuthoredTestFiles {
		return fmt.Errorf("authored test bundle file count exceeds bounds")
	}
	allowed, seen, total := map[string]bool{}, map[string]bool{}, 0
	for _, path := range spec.ApprovedTestPaths {
		allowed[filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))] = true
	}
	for _, file := range bundle.Files {
		if !allowed[file.Path] || seen[file.Path] || file.Bytes != len([]byte(file.Content)) || file.Bytes < 0 {
			return fmt.Errorf("authored test bundle contains an invalid file")
		}
		digest := sha256.Sum256([]byte(file.Content))
		if file.Digest != hex.EncodeToString(digest[:]) {
			return fmt.Errorf("authored test file digest mismatch")
		}
		seen[file.Path], total = true, total+file.Bytes
	}
	if total > budgets.AuthoredTestBytes {
		return fmt.Errorf("authored test bundle byte count exceeds bounds")
	}
	copyFiles := append([]QATestFile(nil), bundle.Files...)
	sort.Slice(copyFiles, func(i, j int) bool { return copyFiles[i].Path < copyFiles[j].Path })
	digest, err := fingerprintQAValue(copyFiles)
	if err != nil || digest != bundle.ContentDigest {
		return fmt.Errorf("authored test bundle content digest mismatch")
	}
	return nil
}

// ClassifyQAReproductionResult accepts a failing result only when the frozen
// test identity and discriminators all match. Infrastructure and unrelated
// failures stay inconclusive.
func ClassifyQAReproductionResult(result QACommandResult, predicted QAFailureSignature) (QAEvidenceOutcome, string) {
	if err := ValidateQAFailureSignature(predicted); err != nil {
		return QAEvidenceInconclusive, "invalid_failure_signature"
	}
	if result.TimedOut {
		return QAEvidenceInconclusive, "timeout"
	}
	if result.Cancelled || !result.CleanupComplete {
		return QAEvidenceInconclusive, "infrastructure_error"
	}
	if result.Truncated {
		return QAEvidenceInconclusive, "output_truncated"
	}
	combined := result.Stdout + "\n" + result.Stderr
	lower := strings.ToLower(combined)
	if strings.Contains(lower, "build failed") || strings.Contains(lower, "undefined:") || strings.Contains(lower, "syntax error") || strings.Contains(lower, "panic:") {
		return QAEvidenceInconclusive, "unrelated_compile_or_panic_failure"
	}
	if result.ExitCode == 0 {
		if predicted.ExitClass == "zero" && signatureOutputMatches(combined, predicted) {
			return QAEvidenceFail, "predicted_failure_reproduced"
		}
		return QAEvidencePass, "expected_behavior_observed"
	}
	if !exitClassMatches(result.ExitCode, predicted.ExitClass) || !signatureOutputMatches(combined, predicted) {
		return QAEvidenceInconclusive, "failure_signature_mismatch"
	}
	return QAEvidenceFail, "predicted_failure_reproduced"
}

func exitClassMatches(exitCode int, class string) bool {
	if class == "nonzero" {
		return exitCode != 0
	}
	if class == "zero" {
		return exitCode == 0
	}
	want, err := strconv.Atoi(strings.TrimPrefix(class, "exit:"))
	return err == nil && exitCode == want
}

func signatureOutputMatches(output string, signature QAFailureSignature) bool {
	if !strings.Contains(output, signature.TestName) || !strings.Contains(output, signature.OutputMatcher) {
		return false
	}
	if signature.ErrorCode != "" && !strings.Contains(output, signature.ErrorCode) {
		return false
	}
	return true
}

func ValidateQAReproductionRun(run QAReproductionRun, spec QAReproductionSpec, bundle QATestBundle) error {
	if run.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(run.ID, "run") || run.SpecID != spec.ID || run.TestBundleID != bundle.ID || !validFingerprint(run.TargetIdentity) || run.CompletedAt.IsZero() {
		return fmt.Errorf("invalid QA reproduction run identity")
	}
	if err := ValidateQAFailureSignature(run.Signature); err != nil || run.Signature != spec.PredictedFailure {
		return fmt.Errorf("QA reproduction run signature does not match its frozen specification")
	}
	if !validEvidenceOutcome(run.Outcome) || strings.TrimSpace(run.ReasonCode) == "" {
		return fmt.Errorf("QA reproduction run outcome is invalid")
	}
	wantOutcome, wantReason := ClassifyQAReproductionResult(run.Result, spec.PredictedFailure)
	if qaReproductionIntegrityOverride(run.ReasonCode) {
		wantOutcome, wantReason = QAEvidenceInconclusive, run.ReasonCode
	}
	legacyMatcherUpgrade := run.Outcome == QAEvidenceInconclusive && run.ReasonCode == "failure_signature_mismatch" && wantOutcome == QAEvidenceFail && wantReason == "predicted_failure_reproduced"
	if !legacyMatcherUpgrade && (run.Outcome != wantOutcome || run.ReasonCode != wantReason) {
		return fmt.Errorf("QA reproduction run classification does not match its command result")
	}
	if !run.Cleanup.Attempted {
		return fmt.Errorf("QA reproduction run cleanup was not attempted")
	}
	if run.ReasonCode == "cleanup_uncertain" && run.Cleanup.Complete {
		return fmt.Errorf("QA reproduction cleanup reason contradicts complete cleanup facts")
	}
	if !run.Cleanup.Complete {
		if run.Outcome != QAEvidenceInconclusive || run.ReasonCode != "cleanup_uncertain" {
			return fmt.Errorf("incomplete QA reproduction cleanup must remain inconclusive")
		}
	} else if !run.Cleanup.DescendantsTerminated || !run.Cleanup.WorkspaceRemoved {
		return fmt.Errorf("complete QA reproduction cleanup lacks required facts")
	}
	return nil
}

func qaReproductionIntegrityOverride(reason string) bool {
	switch reason {
	case "runtime_cleanup_incomplete", "workspace_changes_incomplete", "source_change_detected", "cleanup_uncertain", "target_drift":
		return true
	default:
		return false
	}
}

func ValidateQAIssueEvidenceCoverage(value QAIssueEvidenceCoverage) error {
	if value.SchemaVersion != QAEvidenceSchemaVersion || !validQAV2ID(value.IssueID, "issue") || len(value.TheoryIDs) == 0 || len(value.TestBundleIDs) == 0 || len(value.PrimaryReproducers) == 0 {
		return fmt.Errorf("issue evidence coverage is incomplete")
	}
	tests := map[string]bool{}
	for _, id := range value.TestBundleIDs {
		if !validQAV2ID(id, "test") {
			return fmt.Errorf("issue evidence coverage has an invalid test bundle")
		}
		tests[id] = true
	}
	for _, id := range value.PrimaryReproducers {
		if !tests[id] {
			return fmt.Errorf("primary reproducer is absent from issue test bundles")
		}
	}
	for _, theoryID := range value.TheoryIDs {
		if !validQAIDKind(theoryID, "theory") || len(value.Coverage[theoryID]) == 0 {
			return fmt.Errorf("confirmed theory has no explicit evidence coverage")
		}
	}
	return nil
}
