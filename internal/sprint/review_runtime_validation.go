package sprint

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Antonio7098/agentwrap"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

func extractValidatedReviewResult(root string, manifest ReviewManifest, coverageID string, runtimeResult pruntime.Result) (ReviewCoverageResult, []string) {
	var result ReviewCoverageResult
	if !extractReviewResult(runtimeResult, &result) {
		return result, []string{"runtime did not return a structured review result"}
	}
	normalizeReviewResultForManifest(manifest, &result)
	return result, reviewResultProblems(root, manifest, coverageID, result)
}

const (
	maxReviewFindingsPerCoverage = 50
	maxReviewSummaryBytes        = 4 << 10
	maxReviewFindingTextBytes    = 8 << 10
)

// reviewOutputCapture bridges structured assistant events into the validation
// boundary. Some runtimes retain the final assistant text in events/DB while
// returning an empty TerminalOutput, so terminal-only validation can reject a
// valid result that the platform adapter can recover immediately afterwards.
type reviewOutputCapture struct {
	mu     sync.RWMutex
	result ReviewCoverageResult
	found  bool
}

func (c *reviewOutputCapture) observe(payload map[string]any) {
	var result ReviewCoverageResult
	if !extractReviewValue(payload, &result) {
		return
	}
	c.mu.Lock()
	c.result = result
	c.found = true
	c.mu.Unlock()
}

func (c *reviewOutputCapture) load() (ReviewCoverageResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.result, c.found
}

func (s Service) reviewValidationSpec(m ReviewManifest, coverage ReviewInput, captured *reviewOutputCapture) *agentwrap.ValidationSpec {
	return &agentwrap.ValidationSpec{
		Validators: []agentwrap.Validator{agentwrap.ValidatorFunc(func(ctx context.Context, vctx agentwrap.ValidationContext) agentwrap.ValidationCheck {
			if err := ctx.Err(); err != nil {
				return reviewValidationCheck(coverage.ID, []string{"validation cancelled: " + err.Error()})
			}
			var result ReviewCoverageResult
			found := extractReviewValue(vctx.Result.TerminalOutput, &result)
			if !found && captured != nil {
				result, found = captured.load()
			}
			if !found {
				return reviewValidationCheck(coverage.ID, []string{"terminal output did not contain one review result JSON object"})
			}
			normalizeReviewResultForManifest(m, &result)
			return reviewValidationCheck(coverage.ID, reviewResultProblems(s.root, m, coverage.ID, result))
		})},
		Repair: agentwrap.RepairConfig{
			MaxAttempts:                 2,
			SessionAction:               agentwrap.SessionActionContinue,
			AllowFreshSessionFallback:   true,
			FreshSessionFallbackOnError: true,
			BuildPrompt: func(ctx agentwrap.RepairContext) string {
				return buildReviewRepairPrompt(m, coverage, validationFailureDetails(ctx.Validation.Failures), ctx.CurrentResult.TerminalOutput)
			},
			OverrideRequest: func(ctx agentwrap.RepairContext, req agentwrap.RunRequest) agentwrap.RunRequest {
				// A semantically invalid same-session repair gets one clean retry.
				if ctx.Attempt >= 2 {
					req.SessionID = ""
					req.SessionAction = agentwrap.SessionActionFresh
				}
				return req
			},
		},
	}
}

func reviewValidationCheck(coverageID string, problems []string) agentwrap.ValidationCheck {
	check := agentwrap.ValidationCheck{
		ExpectationID: "review-result-" + coverageID,
		Kind:          agentwrap.ExpectationCustom,
		Severity:      agentwrap.ExpectationRequired,
		Expected:      "one schemaVersion 1 review result for coverageId " + coverageID,
		RepairHint:    "Return only the canonical JSON object; do not perform more tool calls.",
	}
	if len(problems) == 0 {
		check.Passed = true
		check.Observed = "valid"
		check.Detail = "review result passed schema and citation validation"
		return check
	}
	check.Observed = strings.Join(problems, "; ")
	check.Detail = "review result failed semantic validation"
	return check
}

func validationFailureDetails(failures []agentwrap.ValidationFailure) []string {
	var details []string
	for _, failure := range failures {
		var parts []string
		if value := strings.TrimSpace(failure.Observed); value != "" {
			parts = append(parts, "observed: "+value)
		}
		if value := strings.TrimSpace(failure.Detail); value != "" {
			parts = append(parts, "validation: "+value)
		}
		if value := strings.TrimSpace(failure.RepairHint); value != "" {
			parts = append(parts, "repair: "+value)
		}
		if len(parts) > 0 {
			prefix := strings.TrimSpace(failure.ExpectationID)
			if prefix == "" {
				prefix = "review result"
			}
			details = append(details, prefix+": "+strings.Join(parts, "; "))
		}
	}
	return details
}

func buildReviewRepairPrompt(manifest ReviewManifest, coverage ReviewInput, problems []string, priorOutput string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Return only one corrected JSON object for coverageId %q. Do not perform more tool calls. Preserve the prior review's substantive conclusions while correcting its structure and citations.\n", coverage.ID)
	fmt.Fprintln(&b, `Canonical schema: {"schemaVersion":1,"coverageId":string,"applicability":"direct|partial|not_triggered|explicitly_deferred","summary":string,"findings":[{"id":string,"severity":"info|low|medium|high|blocker","applicability":"direct|partial|not_triggered|explicitly_deferred","title":string,"detail":string,"action":string,"citations":[{"path":string,"startLine":number,"endLine":number}]}]}`)
	if len(problems) > 0 {
		fmt.Fprintln(&b, "Validation failures:")
		for _, problem := range problems {
			fmt.Fprintf(&b, "- %s\n", problem)
		}
	}
	fmt.Fprintln(&b, "Allowed citation paths and inclusive line ranges (use the logical path exactly):")
	for _, input := range manifest.Inputs {
		if data, ok := manifest.Contents[input.Path]; ok {
			fmt.Fprintf(&b, "- `%s`: 1-%d (read path `%s`)\n", input.Path, len(strings.Split(data, "\n")), reviewInputReadPath(manifest, input))
		}
	}
	if value := strings.TrimSpace(priorOutput); value != "" {
		const maxPriorOutputBytes = 48 << 10
		if len(value) > maxPriorOutputBytes {
			value = value[len(value)-maxPriorOutputBytes:]
		}
		fmt.Fprintf(&b, "Prior output to correct:\n%s\n", value)
	}
	fmt.Fprintln(&b, "Use findings only for actionable deviations. Put confirmed conformance in summary, not in info findings.")
	return b.String()
}

func normalizeReviewResult(result *ReviewCoverageResult) {
	if result == nil {
		return
	}
	result.Applicability = canonicalReviewApplicability(result.Applicability)
	for i := range result.Findings {
		result.Findings[i].Applicability = canonicalReviewApplicability(result.Findings[i].Applicability)
	}
}

// normalizeReviewResultForManifest accepts the exact read path exposed to a
// reviewer and stores its stable logical manifest path. This keeps persisted
// citations portable without rejecting an otherwise valid reviewer result for
// citing the path it actually read.
func normalizeReviewResultForManifest(manifest ReviewManifest, result *ReviewCoverageResult) {
	normalizeReviewResult(result)
	if result == nil {
		return
	}
	readPaths := make(map[string]string, len(manifest.Inputs))
	for _, input := range manifest.Inputs {
		if path := reviewInputReadPath(manifest, input); path != "<embedded>" {
			readPaths[filepath.Clean(path)] = input.Path
		}
	}
	for findingIndex := range result.Findings {
		for citationIndex := range result.Findings[findingIndex].Citations {
			citation := &result.Findings[findingIndex].Citations[citationIndex]
			if logical, ok := readPaths[filepath.Clean(citation.Path)]; ok {
				citation.Path = logical
			}
		}
	}
}

func canonicalReviewApplicability(value string) string {
	if value == "deferred" { // compatibility with protocol revisions before schema v1 was clarified
		return "explicitly_deferred"
	}
	return value
}

func reviewResultProblems(root string, manifest ReviewManifest, expectedCoverageID string, result ReviewCoverageResult) []string {
	normalizeReviewResultForManifest(manifest, &result)
	var problems []string
	if result.SchemaVersion != 1 {
		problems = append(problems, "schemaVersion must be 1")
	}
	if result.CoverageID != expectedCoverageID {
		problems = append(problems, fmt.Sprintf("coverageId must be %q (got %q)", expectedCoverageID, result.CoverageID))
	}
	if !validReviewApplicability(result.Applicability) {
		problems = append(problems, "result applicability must be direct, partial, not_triggered, or explicitly_deferred")
	}
	if strings.TrimSpace(result.Summary) == "" || len(result.Summary) > maxReviewSummaryBytes {
		problems = append(problems, fmt.Sprintf("summary must be non-empty and at most %d bytes", maxReviewSummaryBytes))
	}
	if len(result.Findings) > maxReviewFindingsPerCoverage {
		problems = append(problems, fmt.Sprintf("findings must contain at most %d actionable items", maxReviewFindingsPerCoverage))
	}
	seen := map[string]bool{}
	for i, finding := range result.Findings {
		prefix := fmt.Sprintf("finding[%d]", i)
		if strings.TrimSpace(finding.ID) == "" || seen[finding.ID] {
			problems = append(problems, prefix+" id must be non-empty and unique")
		}
		seen[finding.ID] = true
		if !validReviewSeverity(finding.Severity) {
			problems = append(problems, prefix+" severity is invalid")
		}
		if !validReviewApplicability(finding.Applicability) {
			problems = append(problems, prefix+" applicability is invalid")
		}
		if strings.TrimSpace(finding.Title) == "" || strings.TrimSpace(finding.Detail) == "" || strings.TrimSpace(finding.Action) == "" || len(finding.Title) > maxReviewFindingTextBytes || len(finding.Detail) > maxReviewFindingTextBytes || len(finding.Action) > maxReviewFindingTextBytes {
			problems = append(problems, prefix+" title, detail, and action must be non-empty and within their size limits")
		}
		if reviewApplicable(finding.Applicability) && len(finding.Citations) == 0 {
			problems = append(problems, prefix+" direct or partial finding requires a citation")
		}
		for citationIndex, citation := range finding.Citations {
			if !validReviewCitation(root, manifest, citation) {
				problems = append(problems, fmt.Sprintf("%s citation[%d] is outside the frozen manifest or line range", prefix, citationIndex))
			}
		}
	}
	if len(problems) > 12 {
		problems = append(problems[:12], fmt.Sprintf("%d additional validation failures omitted", len(problems)-12))
	}
	return problems
}

func validReviewSeverity(value string) bool {
	switch value {
	case "info", "low", "medium", "high", "blocker":
		return true
	default:
		return false
	}
}

func reviewManifestChanges(before, after ReviewManifest, findings []ValidationFinding, prepareErr error) []string {
	var changes []string
	if prepareErr != nil {
		changes = append(changes, "review inputs could not be recomputed: "+prepareErr.Error())
	}
	for _, finding := range findings {
		changes = append(changes, fmt.Sprintf("input validation changed at %s: %s", finding.Path, finding.Problem))
		if len(changes) >= 8 {
			break
		}
	}
	beforeHashes := map[string]string{}
	afterHashes := map[string]string{}
	for _, input := range before.Inputs {
		beforeHashes[input.Path] = input.Hash
	}
	for _, input := range after.Inputs {
		afterHashes[input.Path] = input.Hash
	}
	paths := map[string]bool{}
	for path := range beforeHashes {
		paths[path] = true
	}
	for path := range afterHashes {
		paths[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	for _, path := range ordered {
		if beforeHashes[path] == afterHashes[path] {
			continue
		}
		change := "changed"
		if _, ok := beforeHashes[path]; !ok {
			change = "added"
		} else if _, ok := afterHashes[path]; !ok {
			change = "removed"
		}
		changes = append(changes, fmt.Sprintf("review input %s: %s", change, path))
		if len(changes) >= 8 {
			break
		}
	}
	if len(changes) == 0 && before.Fingerprint != after.Fingerprint {
		changes = append(changes, "review manifest fingerprint changed (input ordering or changed-path scope differs)")
	}
	if len(changes) == 0 {
		changes = append(changes, "review inputs failed the final promotion check")
	}
	return changes
}
