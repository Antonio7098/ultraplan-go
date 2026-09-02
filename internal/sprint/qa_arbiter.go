package sprint

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type qaArbiterOutput struct {
	SchemaVersion int                 `json:"schema_version"`
	Overrides     []QAArbiterOverride `json:"overrides"`
	Issues        []QAArbiterIssue    `json:"issues"`
}

type qaIssueReconcilerOutput struct {
	SchemaVersion int              `json:"schema_version"`
	Issues        []QAArbiterIssue `json:"issues"`
}

type qaTheoryGroupPlan struct {
	ID              string
	Theories        []QATheory
	ContextBlockIDs []string
}

const qaArbiterOutputContract = `Return exactly one JSON object with schema_version 1, overrides, and issues. The first output byte must be "{" and the last output byte must be "}". Do not emit Markdown fences, backticks, prose, or leading or trailing commentary. Every override contains theory_ids, action, outcome, replacement_claim, reason, reason_refs, and numeric confidence from 0 to 1. The only valid action strings are "confirm", "refute", "replace", "merge", "split", "invalidate", and "keep_inconclusive". The only valid outcome strings are "confirmed", "refuted", "invalid", "inconclusive", and "cross_shard". Use action "confirm" for an unchanged confirmed theory and "keep_inconclusive" for an unchanged inconclusive theory. A supplied theory may occur in at most one override. When merging theories, emit one merge override that contains all merged theory_ids; do not also emit separate overrides for those theories. Every issue contains theory_ids, claim, title, issue_class, severity, location, reason, and evidence_refs. Omit id or return it as an empty string; the product assigns it. Every effectively confirmed theory must occur in exactly one issue. Refuted, invalid, inconclusive, and cross_shard theories must occur in none. Use only IDs in the frozen pack's allowed_refs array for theory_ids, reason_refs, and evidence_refs. An ID mentioned inside theory prose or nested evidence is not allowed unless allowed_refs also contains it.`

func (s Service) arbitrateQA(ctx context.Context, qaMap QAMap, shards []QAShard, target string) (QAArbitration, error) {
	if qaMap.Foundation == nil {
		return QAArbitration{}, NewQAError(QAErrorInvalidState, "arbitrate QA", "frozen QA foundation is unavailable", nil)
	}
	settings, err := s.effectiveQASettings()
	if err != nil {
		return QAArbitration{}, err
	}
	limit := qaMap.Budgets.ArbiterMaxTheories
	if limit <= 0 {
		limit = 24
	}
	groups, err := groupQATheories(qaMap, shards, limit)
	if err != nil {
		return QAArbitration{}, NewQAError(QAErrorInvalidState, "arbitrate QA", "cannot construct bounded theory groups", err)
	}
	if len(groups) == 0 {
		return QAArbitration{SchemaVersion: QASchemaVersion, MapID: qaMap.ID}, nil
	}
	arbitration := QAArbitration{SchemaVersion: QASchemaVersion, MapID: qaMap.ID}
	for _, group := range groups {
		record, groupErr := s.runQAArbiterGroup(ctx, qaMap, group, target, settings)
		if groupErr != nil {
			return QAArbitration{}, groupErr
		}
		arbitration.Groups = append(arbitration.Groups, record)
		arbitration.Overrides = append(arbitration.Overrides, record.Overrides...)
		if arbitration.Model == "" {
			arbitration.Model = record.Model
		}
	}
	var provisional []QAArbiterIssue
	for _, group := range arbitration.Groups {
		provisional = append(provisional, group.Issues...)
	}
	arbitration.Issues, arbitration.Reconciliation, err = s.reconcileQAArbiterIssues(ctx, qaMap, provisional, target, settings)
	if err != nil {
		return QAArbitration{}, err
	}
	return arbitration, nil
}

func groupQATheories(qaMap QAMap, shards []QAShard, maxTheories int) ([]qaTheoryGroupPlan, error) {
	if maxTheories < 1 {
		return nil, fmt.Errorf("arbiter maximum theory size must be positive")
	}
	var theories []QATheory
	shardByTheory := make(map[string]QAShard)
	for _, shard := range shards {
		for _, theory := range shard.Theories {
			theories = append(theories, theory)
			shardByTheory[theory.ID] = shard
		}
	}
	sort.Slice(theories, func(i, j int) bool { return theories[i].ID < theories[j].ID })
	unassigned := make(map[string]QATheory, len(theories))
	for _, theory := range theories {
		unassigned[theory.ID] = theory
	}
	var groups []qaTheoryGroupPlan
	for len(unassigned) > 0 {
		seed, bestDensity := "", -1
		for id, theory := range unassigned {
			density := 0
			for otherID, other := range unassigned {
				if id != otherID {
					density += qaTheoryAffinity(theory, shardByTheory[id], other, shardByTheory[otherID])
				}
			}
			if density > bestDensity || density == bestDensity && (seed == "" || id < seed) {
				seed, bestDensity = id, density
			}
		}
		selected := []QATheory{unassigned[seed]}
		delete(unassigned, seed)
		for len(selected) < maxTheories && len(unassigned) > 0 {
			candidateID, bestScore := "", 0
			for id, candidate := range unassigned {
				score := 0
				for _, member := range selected {
					score += qaTheoryAffinity(candidate, shardByTheory[id], member, shardByTheory[member.ID])
				}
				if score > bestScore || score == bestScore && score > 0 && (candidateID == "" || id < candidateID) {
					candidateID, bestScore = id, score
				}
			}
			if candidateID == "" {
				break
			}
			selected = append(selected, unassigned[candidateID])
			delete(unassigned, candidateID)
		}
		theoryIDs, blockIDs := qaTheoryIDs(selected), []string{}
		for _, theory := range selected {
			blockIDs = append(blockIDs, shardByTheory[theory.ID].ContextBlockIDs...)
			for _, block := range qaMap.Foundation.Blocks {
				if qaTheoryCitesBlock(theory, block.ID) {
					blockIDs = append(blockIDs, block.ID)
				}
			}
		}
		identity, err := fingerprintQAValue(struct {
			Map      string
			Theories []string
		}{qaMap.ID, theoryIDs})
		if err != nil {
			return nil, err
		}
		groups = append(groups, qaTheoryGroupPlan{ID: QAIDScope + "-arbiter-group-" + identity[:24], Theories: selected, ContextBlockIDs: normalizeQAStrings(blockIDs)})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].ID < groups[j].ID })
	return groups, nil
}

func qaTheoryAffinity(left QATheory, leftShard QAShard, right QATheory, rightShard QAShard) int {
	score := 12 * sharedQAStrings(leftShard.ContextBlockIDs, rightShard.ContextBlockIDs)
	leftPaths := normalizeQAStrings(append(append(append([]string(nil), leftShard.ChangedPaths...), leftShard.ContextPaths...), leftShard.OverlapPaths...))
	rightPaths := normalizeQAStrings(append(append(append([]string(nil), rightShard.ChangedPaths...), rightShard.ContextPaths...), rightShard.OverlapPaths...))
	score += 7 * sharedQAStrings(leftPaths, rightPaths)
	score += 4 * sharedQAStrings(left.ExpectationRefs, right.ExpectationRefs)
	score += 2 * sharedQAStrings(leftShard.RiskTags, rightShard.RiskTags)
	if left.ShardID == right.ShardID {
		score += 3
	}
	return score
}

func sharedQAStrings(left, right []string) int {
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		seen[value] = struct{}{}
	}
	count := 0
	for _, value := range right {
		if _, ok := seen[value]; ok {
			count++
			delete(seen, value)
		}
	}
	return count
}

func qaTheoryCitesBlock(theory QATheory, blockID string) bool {
	text := theory.Claim + "\n" + theory.Basis + "\n" + theory.VerificationSurface + "\n" + theory.OutcomeReason
	for _, evidence := range theory.Evidence {
		text += "\n" + evidence.Summary + "\n" + evidence.OutputDigest
	}
	return strings.Contains(text, blockID)
}

func (s Service) runQAArbiterGroup(ctx context.Context, qaMap QAMap, group qaTheoryGroupPlan, target string, settings QASettings) (QAArbiterGroup, error) {
	runtimeSettings := settings.RuntimeFor("arbiter")
	provider, model := splitProviderModel(runtimeSettings.Model)
	record := QAArbiterGroup{ID: group.ID, TheoryIDs: qaTheoryIDs(group.Theories), ContextBlockIDs: append([]string(nil), group.ContextBlockIDs...), Model: provider + "/" + model}
	commonBlocks, groupBlocks := splitQAArbiterBlocks(qaMap.Foundation, group.ContextBlockIDs)
	common, err := canonicalQAJSON(struct {
		SchemaVersion int              `json:"schema_version"`
		FoundationID  string           `json:"foundation_id"`
		Fingerprint   string           `json:"fingerprint"`
		Blocks        []QAContextBlock `json:"blocks"`
	}{QASchemaVersion, qaMap.Foundation.ID, qaMap.Foundation.Fingerprint, commonBlocks})
	if err != nil {
		return QAArbiterGroup{}, NewQAError(QAErrorInvalidState, "arbiter group", "cannot encode common arbiter foundation", err)
	}
	allowedRefs := qaTheoryIDs(group.Theories)
	for _, block := range append(append([]QAContextBlock(nil), commonBlocks...), groupBlocks...) {
		allowedRefs = append(allowedRefs, block.ID)
	}
	allowedRefs = normalizeQAStrings(allowedRefs)
	packet, err := canonicalQAJSON(struct {
		GroupID     string           `json:"group_id"`
		MapID       string           `json:"map_id"`
		AllowedRefs []string         `json:"allowed_refs"`
		Blocks      []QAContextBlock `json:"context_blocks"`
		Theories    []QATheory       `json:"theories"`
	}{group.ID, qaMap.ID, allowedRefs, groupBlocks, group.Theories})
	if err != nil {
		return QAArbiterGroup{}, NewQAError(QAErrorInvalidState, "arbiter group", "cannot encode arbiter group", err)
	}
	prefix := `# QA theory-group arbiter

Assess one bounded theory group. Confirm, refute, replace, merge, split, invalidate, or retain inconclusive theories. Group confirmed theories that describe one defect into one issue. Do not invent evidence, requirements, blocks, paths, or checks. Do not authorize a patch or weaken a criterion.

` + qaArbiterOutputContract + `

Common frozen QA foundation:
` + string(common) + "\n\n<<< END STABLE QA ARBITER PREFIX >>>\n"
	prompt := prefix + "\nFrozen arbiter group pack:\n" + string(packet) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		return QAArbiterGroup{}, NewQAError(QAErrorBudgetExhausted, "arbiter group", "arbiter group exceeded the frozen prompt budget", nil)
	}
	req := s.runtimeRequest(prompt, map[string]string{"project": qaMap.Project, "sprint": qaMap.Sprint, "stage": string(VerificationPhaseQA), "map": qaMap.ID, "role": "arbiter", "arbiter_group": group.ID})
	req.Metadata["operation"] = "qa-arbitrate"
	req.Metadata["task"] = group.ID
	req.Metadata["qa_attempt"] = qaMap.SemanticAttemptID
	req.Provider, req.Model = provider, model
	req.Metadata["variant"], req.Metadata["reasoning_effort"] = runtimeSettings.Variant, runtimeSettings.Variant
	req.WorkDir, req.Timeout, req.Sandbox, req.Permissions = filepath.Clean(target), settings.Budgets.ShardTimeout, "read_only", "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = qaReadOnlyToolPolicy()
	req.Cache = pruntime.CacheDirective{Key: "qa-arbiter/" + qaMap.Foundation.Fingerprint + "/" + provider + "/" + model + "/" + runtimeSettings.Variant, BreakpointBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix)), Mode: "stable-prefix"}
	result, runErr := s.startQARuntime(ctx, qaMap, req)
	var output qaArbiterOutput
	decodeErr := decodeStrictQAJSON(result.TerminalOutput, &output)
	if runErr == nil && decodeErr == nil {
		if validated, validationErr := validateQAArbiterGroupOutput(qaMap, group, commonBlocks, groupBlocks, output, record); validationErr == nil {
			return validated, nil
		} else {
			decodeErr = validationErr
		}
	}
	if result.SessionID != "" && ctx.Err() == nil {
		repair := req
		repair.Prompt = "Your arbiter output was rejected: " + safeError(decodeErr) + ". Correct that rejection without dropping any other valid issue coverage or violating another contract rule. " + qaArbiterOutputContract + "\n"
		repair.SessionID, repair.SessionAction, repair.Cache = result.SessionID, "continue", pruntime.CacheDirective{}
		repair.Metadata["repair_of"] = result.RunID
		repaired, repairErr := s.startQARuntime(ctx, qaMap, repair)
		repairDecodeErr := decodeStrictQAJSON(repaired.TerminalOutput, &output)
		if repairErr == nil && repairDecodeErr == nil {
			if validated, validationErr := validateQAArbiterGroupOutput(qaMap, group, commonBlocks, groupBlocks, output, record); validationErr == nil {
				return validated, nil
			} else {
				repairDecodeErr = validationErr
			}
		}
		decodeErr = errors.Join(decodeErr, repairErr, repairDecodeErr)
	}
	return QAArbiterGroup{}, NewQAError(QAErrorInvalidState, "arbiter group", "arbiter agent did not produce valid output", errors.Join(runErr, decodeErr))
}

func splitQAArbiterBlocks(foundation *QAFoundation, selected []string) (common, specific []QAContextBlock) {
	selectedSet := make(map[string]bool, len(selected))
	for _, id := range selected {
		selectedSet[id] = true
	}
	for _, block := range foundation.Blocks {
		if block.Kind == "authority" || block.Kind == "evidence" {
			common = append(common, block)
		} else if selectedSet[block.ID] {
			specific = append(specific, block)
		}
	}
	return common, specific
}

func validateQAArbiterGroupOutput(qaMap QAMap, group qaTheoryGroupPlan, common, specific []QAContextBlock, output qaArbiterOutput, base QAArbiterGroup) (QAArbiterGroup, error) {
	if output.SchemaVersion != QASchemaVersion || len(output.Overrides) > len(group.Theories) || len(output.Issues) > len(group.Theories) {
		return QAArbiterGroup{}, fmt.Errorf("arbiter group schema or output count is invalid")
	}
	refs, outcomes := map[string]bool{}, map[string]QATheoryOutcome{}
	for _, theory := range group.Theories {
		refs[theory.ID], outcomes[theory.ID] = true, theory.Outcome
	}
	for _, block := range append(append([]QAContextBlock(nil), common...), specific...) {
		refs[block.ID] = true
	}
	seenOverride := map[string]bool{}
	for i := range output.Overrides {
		override := &output.Overrides[i]
		override.TheoryIDs, override.ReasonRefs = normalizeQAStrings(override.TheoryIDs), normalizeQAStrings(override.ReasonRefs)
		if len(override.TheoryIDs) == 0 || len(override.ReasonRefs) == 0 || strings.TrimSpace(override.Reason) == "" || override.Confidence < 0 || override.Confidence > 1 {
			return QAArbiterGroup{}, fmt.Errorf("arbiter override is incomplete")
		}
		switch override.Action {
		case QAArbiterConfirm, QAArbiterRefute, QAArbiterReplace, QAArbiterMerge, QAArbiterSplit, QAArbiterInvalidate, QAArbiterKeepInconclusive:
		default:
			return QAArbiterGroup{}, fmt.Errorf("arbiter action %q is invalid", override.Action)
		}
		if override.Outcome != QATheoryConfirmed && override.Outcome != QATheoryRefuted && override.Outcome != QATheoryInvalid && override.Outcome != QATheoryInconclusive && override.Outcome != QATheoryCrossShard {
			return QAArbiterGroup{}, fmt.Errorf("arbiter outcome %q is invalid", override.Outcome)
		}
		for _, id := range override.TheoryIDs {
			if !refs[id] || seenOverride[id] {
				return QAArbiterGroup{}, fmt.Errorf("arbiter theory reference is unknown or superseded twice")
			}
			seenOverride[id], outcomes[id] = true, override.Outcome
		}
		for _, id := range override.ReasonRefs {
			if !refs[id] {
				return QAArbiterGroup{}, fmt.Errorf("arbiter reason reference %q was not delivered", id)
			}
		}
		identity, err := fingerprintQAValue(struct {
			Map, Group string
			Theories   []string
			Action     QAArbiterAction
			Outcome    QATheoryOutcome
			Reason     string
			Refs       []string
		}{qaMap.ID, group.ID, override.TheoryIDs, override.Action, override.Outcome, override.Reason, override.ReasonRefs})
		if err != nil {
			return QAArbiterGroup{}, err
		}
		override.ID = QAIDScope + "-override-" + identity[:24]
	}
	issues, err := validateQAArbiterIssues(qaMap, group.ID, output.Issues, refs, outcomes, true)
	if err != nil {
		return QAArbiterGroup{}, err
	}
	base.Overrides, base.Issues = output.Overrides, issues
	return base, nil
}

func validateQAArbiterIssues(qaMap QAMap, scope string, issues []QAArbiterIssue, refs map[string]bool, outcomes map[string]QATheoryOutcome, requireConfirmedCoverage bool) ([]QAArbiterIssue, error) {
	seen := map[string]bool{}
	for i := range issues {
		issue := &issues[i]
		issue.TheoryIDs, issue.EvidenceRefs = normalizeQAStrings(issue.TheoryIDs), normalizeQAStrings(issue.EvidenceRefs)
		issue.Claim, issue.Title, issue.IssueClass = strings.TrimSpace(issue.Claim), strings.TrimSpace(issue.Title), strings.TrimSpace(issue.IssueClass)
		issue.Severity, issue.Location, issue.Reason = normalizeQASeverity(issue.Severity), normalizeIssueLocation(issue.Location), strings.TrimSpace(issue.Reason)
		if len(issue.TheoryIDs) == 0 || issue.Claim == "" || issue.Title == "" || issue.IssueClass == "" || issue.Location == "" || issue.Reason == "" || len(issue.EvidenceRefs) == 0 {
			return nil, fmt.Errorf("arbiter issue is incomplete")
		}
		for _, id := range issue.TheoryIDs {
			if !refs[id] || seen[id] || outcomes[id] != QATheoryConfirmed {
				return nil, fmt.Errorf("arbiter issue theory is unknown, repeated, or not confirmed")
			}
			seen[id] = true
		}
		for _, id := range issue.EvidenceRefs {
			if !refs[id] {
				return nil, fmt.Errorf("arbiter issue evidence reference %q was not delivered", id)
			}
		}
		identity, err := fingerprintQAValue(struct {
			Map, Scope, Claim, Class, Location string
			Theories, Evidence                 []string
		}{qaMap.ID, scope, issue.Claim, issue.IssueClass, issue.Location, issue.TheoryIDs, issue.EvidenceRefs})
		if err != nil {
			return nil, err
		}
		issue.ID = QAIDScope + "-arbiter-issue-" + identity[:24]
	}
	if requireConfirmedCoverage {
		for id, outcome := range outcomes {
			if outcome == QATheoryConfirmed && !seen[id] {
				return nil, fmt.Errorf("confirmed theory %q is not assigned to an arbiter issue", id)
			}
		}
	}
	sort.Slice(issues, func(i, j int) bool { return issues[i].ID < issues[j].ID })
	return issues, nil
}

func (s Service) reconcileQAArbiterIssues(ctx context.Context, qaMap QAMap, provisional []QAArbiterIssue, target string, settings QASettings) ([]QAArbiterIssue, *QAIssueReconciliation, error) {
	runtimeSettings := settings.RuntimeFor("reconciler")
	provider, model := splitProviderModel(runtimeSettings.Model)
	record := &QAIssueReconciliation{Model: provider + "/" + model}
	if len(provisional) == 0 {
		return nil, record, nil
	}
	packet, err := canonicalQAJSON(struct {
		MapID  string           `json:"map_id"`
		Issues []QAArbiterIssue `json:"provisional_issues"`
	}{qaMap.ID, provisional})
	if err != nil {
		return nil, record, NewQAError(QAErrorInvalidState, "reconcile QA issues", "cannot encode issue reconciliation packet", err)
	}
	prefix := `# QA issue reconciliation agent

Reconcile provisional issues produced by independent arbiter groups. Deduplicate equivalent defects across groups, combine theories that describe one root cause, and split only when one provisional issue contains distinct defects. Preserve every supplied theory exactly once. Do not change theory outcomes, invent evidence, weaken criteria, or authorize repairs.

Return exactly one JSON object with schema_version 1 and issues. Every issue contains theory_ids, claim, title, issue_class, severity, location, reason, and evidence_refs. Omit id or return it empty; the product assigns it. Use only supplied theory and evidence references.

<<< END STABLE QA ISSUE RECONCILIATION PREFIX >>>
`
	prompt := prefix + "\nFrozen provisional issue packet:\n" + string(packet) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		return nil, record, NewQAError(QAErrorBudgetExhausted, "reconcile QA issues", "issue reconciliation prompt exceeded the frozen prompt budget", nil)
	}
	req := s.runtimeRequest(prompt, map[string]string{"project": qaMap.Project, "sprint": qaMap.Sprint, "stage": string(VerificationPhaseQA), "map": qaMap.ID, "role": "issue_reconciler"})
	req.Metadata["operation"] = "qa-reconcile-issues"
	req.Metadata["task"] = qaMap.ID
	req.Metadata["qa_attempt"] = qaMap.SemanticAttemptID
	req.Provider, req.Model = provider, model
	req.Metadata["variant"], req.Metadata["reasoning_effort"] = runtimeSettings.Variant, runtimeSettings.Variant
	req.WorkDir, req.Timeout, req.Sandbox, req.Permissions = filepath.Clean(target), settings.Budgets.ShardTimeout, "read_only", "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = qaReadOnlyToolPolicy()
	req.Cache = pruntime.CacheDirective{Key: "qa-issue-reconciler/" + qaMap.Foundation.Fingerprint + "/" + provider + "/" + model + "/" + runtimeSettings.Variant, BreakpointBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix)), Mode: "stable-prefix"}
	result, runErr := s.startQARuntime(ctx, qaMap, req)
	var output qaIssueReconcilerOutput
	decodeErr := decodeStrictQAJSON(result.TerminalOutput, &output)
	if runErr == nil && decodeErr == nil {
		if issues, validationErr := validateQAReconcilerOutput(qaMap, provisional, output); validationErr == nil {
			return issues, record, nil
		} else {
			decodeErr = validationErr
		}
	}
	if result.SessionID != "" && ctx.Err() == nil {
		repair := req
		repair.Prompt = "Your issue reconciliation output was rejected: " + safeError(decodeErr) + ". Return only corrected JSON using supplied references.\n"
		repair.SessionID, repair.SessionAction, repair.Cache = result.SessionID, "continue", pruntime.CacheDirective{}
		repair.Metadata["repair_of"] = result.RunID
		repaired, repairErr := s.startQARuntime(ctx, qaMap, repair)
		if repairErr == nil && decodeStrictQAJSON(repaired.TerminalOutput, &output) == nil {
			if issues, validationErr := validateQAReconcilerOutput(qaMap, provisional, output); validationErr == nil {
				return issues, record, nil
			}
		}
	}
	return nil, record, NewQAError(QAErrorInvalidState, "reconcile QA issues", "issue reconciliation agent did not produce valid output", errors.Join(runErr, decodeErr))
}

func validateQAReconcilerOutput(qaMap QAMap, provisional []QAArbiterIssue, output qaIssueReconcilerOutput) ([]QAArbiterIssue, error) {
	theoryCount := 0
	for _, issue := range provisional {
		theoryCount += len(issue.TheoryIDs)
	}
	if output.SchemaVersion != QASchemaVersion || len(output.Issues) == 0 || len(output.Issues) > theoryCount {
		return nil, fmt.Errorf("issue reconciliation schema or issue count is invalid")
	}
	refs, outcomes := map[string]bool{}, map[string]QATheoryOutcome{}
	for _, issue := range provisional {
		for _, id := range issue.TheoryIDs {
			refs[id], outcomes[id] = true, QATheoryConfirmed
		}
		for _, id := range issue.EvidenceRefs {
			refs[id] = true
		}
	}
	return validateQAArbiterIssues(qaMap, "reconciled", output.Issues, refs, outcomes, true)
}

func deterministicQAArbiterIssueReconciliation(qaMap QAMap, provisional []QAArbiterIssue) []QAArbiterIssue {
	byKey := map[string]*QAArbiterIssue{}
	for _, candidate := range provisional {
		key := strings.ToLower(strings.TrimSpace(candidate.Claim)) + "\x00" + strings.ToLower(strings.TrimSpace(candidate.IssueClass)) + "\x00" + normalizeIssueLocation(candidate.Location)
		current := byKey[key]
		if current == nil {
			copy := candidate
			copy.ID = ""
			byKey[key] = &copy
			continue
		}
		current.TheoryIDs = normalizeQAStrings(append(current.TheoryIDs, candidate.TheoryIDs...))
		current.EvidenceRefs = normalizeQAStrings(append(current.EvidenceRefs, candidate.EvidenceRefs...))
		if qaSeverityRank(candidate.Severity) > qaSeverityRank(current.Severity) {
			current.Severity = candidate.Severity
		}
		if candidate.Title < current.Title {
			current.Title = candidate.Title
		}
		current.Reason = "deterministic reconciliation of equivalent arbiter issues"
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]QAArbiterIssue, 0, len(keys))
	for _, key := range keys {
		issue := *byKey[key]
		identity, _ := fingerprintQAValue(struct {
			Map, Key           string
			Theories, Evidence []string
		}{qaMap.ID, key, issue.TheoryIDs, issue.EvidenceRefs})
		issue.ID = QAIDScope + "-arbiter-issue-" + identity[:24]
		result = append(result, issue)
	}
	return result
}

func qaTheoryIDs(theories []QATheory) []string {
	ids := make([]string, 0, len(theories))
	for _, theory := range theories {
		ids = append(ids, theory.ID)
	}
	return normalizeQAStrings(ids)
}

func applyQAArbitration(shards []QAShard, arbitration QAArbitration) []QAShard {
	result := append([]QAShard(nil), shards...)
	outcomes, reasons := map[string]QATheoryOutcome{}, map[string]string{}
	for _, override := range arbitration.Overrides {
		for _, theoryID := range override.TheoryIDs {
			outcomes[theoryID] = override.Outcome
			reasons[theoryID] = "Superseded by " + override.ID + ": " + strings.TrimSpace(override.Reason)
		}
	}
	for i := range result {
		result[i].Theories = append([]QATheory(nil), result[i].Theories...)
		for j := range result[i].Theories {
			if outcome, ok := outcomes[result[i].Theories[j].ID]; ok {
				result[i].Theories[j].Outcome = outcome
				result[i].Theories[j].OutcomeReason = reasons[result[i].Theories[j].ID]
			}
		}
	}
	return result
}
