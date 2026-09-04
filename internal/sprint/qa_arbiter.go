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
	SchemaVersion    int                        `json:"schema_version"`
	Overrides        []QAArbiterOverride        `json:"overrides"`
	Issues           []QAArbiterIssue           `json:"issues"`
	EvidenceRequests []QAArbiterEvidenceRequest `json:"evidence_requests"`
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

const qaArbiterOutputContract = `Return exactly one JSON object with schema_version 1, overrides, issues, and evidence_requests. The first output byte must be "{" and the last output byte must be "}". Do not emit Markdown fences, backticks, prose, or leading or trailing commentary. Every override contains theory_ids, action, outcome, replacement_claim, reason, reason_refs, and numeric confidence from 0 to 1. The only valid action strings are "confirm", "refute", "replace", "merge", "split", "invalidate", and "keep_inconclusive". The only valid outcome strings are "confirmed", "refuted", "invalid", "inconclusive", and "cross_shard". Use action "confirm" for an unchanged confirmed theory and "keep_inconclusive" for an unchanged inconclusive theory. A supplied theory may occur in at most one override. When merging theories, emit one merge override that contains all merged theory_ids; do not also emit separate overrides for those theories. Every issue contains theory_ids, claim, title, issue_class, severity, location, reason, and evidence_refs. Omit id or return it as an empty string; the product assigns it. Every effectively confirmed theory must occur in exactly one issue. Refuted, invalid, inconclusive, and cross_shard theories must occur in none. Every evidence request contains theory_ids, origin_shard_id, gap, requested_evidence, required_observation, control_requirement, and priority. Priority must be a JSON string with exactly one of these values: "high", "medium", or "low". Omit id and arbiter_group_id or return them empty; the product assigns both. A request may contain theories from exactly one origin shard and only when their current evidence is insufficient. Split requests by origin shard. Use only IDs in the frozen pack's allowed_refs array for theory_ids, reason_refs, and evidence_refs. An ID mentioned inside theory prose or nested evidence is not allowed unless allowed_refs also contains it.`

func qaArbiterEvidenceRequestInstructions(qaMap QAMap) string {
	return "Evidence-request limits: at most " + fmt.Sprintf("%d", qaMap.Budgets.EvidenceRoundsPerShard) + ` requests per origin shard. Every request must ask the original investigator to create or strengthen an executable Go _test.go reproducer in its approved private workspace. Do not request another source excerpt, listing, search, review, or prose explanation. Combine related evidence gaps into one focused test request when necessary to remain within the limit. Put only supplied theory IDs in reason_refs and evidence_refs. Do not put block IDs in those arrays; discuss block evidence in reason text instead.`
}

func (s Service) arbitrateQA(ctx context.Context, qaMap QAMap, shards []QAShard, target string) (QAArbitration, error) {
	return s.arbitrateQAAffected(ctx, qaMap, shards, target, nil, nil)
}

func (s Service) arbitrateQAAffected(ctx context.Context, qaMap QAMap, shards []QAShard, target string, previous *QAArbitration, affectedTheories map[string]bool) (QAArbitration, error) {
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
	previousGroups := map[string]QAArbiterGroup{}
	if previous != nil {
		for _, group := range previous.Groups {
			previousGroups[group.ID] = group
		}
	}
	for _, group := range groups {
		record, retained := previousGroups[group.ID]
		if !retained || qaTheoryGroupAffected(group, affectedTheories) {
			var groupErr error
			var prior *QAArbiterGroup
			if retained {
				prior = &record
			}
			record, groupErr = s.runQAArbiterGroup(ctx, qaMap, group, target, settings, prior)
			if groupErr != nil {
				return QAArbitration{}, groupErr
			}
		}
		arbitration.Groups = append(arbitration.Groups, record)
		arbitration.Overrides = append(arbitration.Overrides, record.Overrides...)
		arbitration.EvidenceRequests = append(arbitration.EvidenceRequests, record.EvidenceRequests...)
		if arbitration.Model == "" {
			arbitration.Model = record.Model
		}
	}
	var provisional []QAArbiterIssue
	for _, group := range arbitration.Groups {
		provisional = append(provisional, group.Issues...)
	}
	if len(arbitration.EvidenceRequests) > 0 {
		// Cross-group reconciliation is final-only. Provisional issues remain on
		// their group records while original investigators strengthen evidence.
		return arbitration, nil
	}
	arbitration.Issues, arbitration.Reconciliation, err = s.reconcileQAArbiterIssues(ctx, qaMap, provisional, target, settings)
	if err != nil {
		return QAArbitration{}, err
	}
	return arbitration, nil
}

func qaTheoryGroupAffected(group qaTheoryGroupPlan, affected map[string]bool) bool {
	if len(affected) == 0 {
		return false
	}
	for _, theory := range group.Theories {
		if affected[theory.ID] {
			return true
		}
	}
	return false
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

func (s Service) runQAArbiterGroup(ctx context.Context, qaMap QAMap, group qaTheoryGroupPlan, target string, settings QASettings, previous *QAArbiterGroup) (QAArbiterGroup, error) {
	runtimeSettings := settings.RuntimeFor("arbiter")
	provider, model := splitProviderModel(runtimeSettings.Model)
	record := QAArbiterGroup{ID: group.ID, TheoryIDs: qaTheoryIDs(group.Theories), ContextBlockIDs: append([]string(nil), group.ContextBlockIDs...), Provider: provider, Model: provider + "/" + model, Variant: runtimeSettings.Variant, Round: 1}
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

` + qaArbiterOutputContract + "\n\n" + qaArbiterEvidenceRequestInstructions(qaMap) + `

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
	if previous != nil {
		// The retained store is part of the arbiter's identity. Restore it before
		// validating the continuation request because runtimeRequest may derive a
		// different default after an upgrade or configuration change.
		req.RuntimeStorePath = previous.RuntimeStoreRef
		if err := validateRetainedQAArbiterIdentity(*previous, group, req); err != nil {
			return QAArbiterGroup{}, NewQAError(QAErrorRuntimeUnavailable, "re-arbitrate QA group", "original_arbiter_session_unavailable", err)
		}
		record.Round = previous.Round + 1
		req.SessionID, req.SessionAction = previous.SessionID, "continue"
		req.Cache = pruntime.CacheDirective{}
		req.Metadata["arbiter_round"] = fmt.Sprintf("%d", record.Round)
		req.Metadata["evidence_return"] = "true"
		req.Prompt = "New evidence requested by this arbiter group is now available. Reassess the same theory group using the updated frozen pack. Preserve prior decisions unless the new evidence changes them.\n\n" + prompt
	} else {
		req.Metadata["arbiter_round"] = "1"
	}
	if len(req.Prompt) > qaMap.Budgets.PromptBytes {
		return QAArbiterGroup{}, NewQAError(QAErrorBudgetExhausted, "arbiter group", "arbiter continuation exceeded the frozen prompt budget", nil)
	}
	result, runErr := s.startQARuntime(ctx, qaMap, req)
	boundRecord, identityErr := retainQAArbiterRuntimeIdentity(record, req, result, previous)
	if identityErr != nil {
		return QAArbiterGroup{}, NewQAError(QAErrorRuntimeUnavailable, "arbitrate QA group", "original_arbiter_session_unavailable", identityErr)
	}
	record = boundRecord
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
		repair.Prompt = "Your arbiter output was rejected: " + safeError(decodeErr) + ". Correct that rejection without dropping any other valid issue coverage or violating another contract rule. " + qaArbiterOutputContract + "\n\n" + qaArbiterEvidenceRequestInstructions(qaMap) + "\n"
		repair.SessionID, repair.SessionAction, repair.Cache = result.SessionID, "continue", pruntime.CacheDirective{}
		repair.Metadata["repair_of"] = result.RunID
		repaired, repairErr := s.startQARuntime(ctx, qaMap, repair)
		repairedRecord, repairIdentityErr := retainQAArbiterRuntimeIdentity(record, repair, repaired, &record)
		repairDecodeErr := decodeStrictQAJSON(repaired.TerminalOutput, &output)
		if repairErr == nil && repairIdentityErr == nil && repairDecodeErr == nil {
			if validated, validationErr := validateQAArbiterGroupOutput(qaMap, group, commonBlocks, groupBlocks, output, repairedRecord); validationErr == nil {
				return validated, nil
			} else {
				repairDecodeErr = validationErr
			}
		}
		decodeErr = errors.Join(decodeErr, repairErr, repairIdentityErr, repairDecodeErr)
	}
	summary := "arbiter agent did not produce valid output"
	if reason := safeError(decodeErr); reason != "" {
		summary += ": " + reason
	}
	return QAArbiterGroup{}, NewQAError(QAErrorInvalidState, "arbiter group", summary, errors.Join(runErr, decodeErr))
}

func validateRetainedQAArbiterIdentity(previous QAArbiterGroup, group qaTheoryGroupPlan, req pruntime.Request) error {
	theoryIDs := qaTheoryIDs(group.Theories)
	if previous.ID != group.ID || len(previous.TheoryIDs) != len(theoryIDs) || sharedQAStrings(previous.TheoryIDs, theoryIDs) != len(theoryIDs) {
		return fmt.Errorf("retained arbiter group identity changed")
	}
	if previous.SessionID == "" || previous.Provider != req.Provider || previous.Model != req.Provider+"/"+req.Model || previous.Variant != req.Metadata["variant"] || previous.RuntimeStoreRef != req.RuntimeStorePath || previous.WorkspaceID != hashOpaque(filepath.Clean(req.WorkDir)) || previous.Round < 1 {
		return fmt.Errorf("retained arbiter runtime identity changed")
	}
	return nil
}

func retainQAArbiterRuntimeIdentity(record QAArbiterGroup, req pruntime.Request, result pruntime.Result, previous *QAArbiterGroup) (QAArbiterGroup, error) {
	if result.SessionID == "" {
		return QAArbiterGroup{}, fmt.Errorf("arbiter runtime returned no resumable session")
	}
	if previous != nil && result.SessionID != previous.SessionID {
		return QAArbiterGroup{}, fmt.Errorf("arbiter runtime replaced the retained session")
	}
	runtimeStoreRef := result.RuntimeStorePath
	if runtimeStoreRef == "" {
		runtimeStoreRef = req.RuntimeStorePath
	}
	if previous != nil && runtimeStoreRef != previous.RuntimeStoreRef {
		return QAArbiterGroup{}, fmt.Errorf("arbiter runtime store changed")
	}
	record.SessionID = result.SessionID
	record.Provider = req.Provider
	record.Model = req.Provider + "/" + req.Model
	record.Variant = req.Metadata["variant"]
	record.RuntimeStoreRef = runtimeStoreRef
	record.WorkspaceID = hashOpaque(filepath.Clean(req.WorkDir))
	return record, nil
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
	if output.SchemaVersion != QASchemaVersion || len(output.Overrides) > len(group.Theories) || len(output.Issues) > len(group.Theories) || len(output.EvidenceRequests) > len(group.Theories) {
		return QAArbiterGroup{}, fmt.Errorf("arbiter group schema or output count is invalid")
	}
	refs, outcomes := map[string]bool{}, map[string]QATheoryOutcome{}
	for _, theory := range group.Theories {
		refs[theory.ID], outcomes[theory.ID] = true, theory.Outcome
	}
	for _, block := range append(append([]QAContextBlock(nil), common...), specific...) {
		refs[block.ID] = true
	}
	normalizeQAArbiterReferences(&output, refs)
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
	output.Issues = retainConfirmedQAArbiterIssueTheories(output.Issues, outcomes)
	issues, err := validateQAArbiterIssues(qaMap, group.ID, output.Issues, refs, outcomes, true)
	if err != nil {
		return QAArbiterGroup{}, err
	}
	output.EvidenceRequests = discardResolvedQAArbiterEvidenceRequests(output.EvidenceRequests, outcomes)
	requests, err := validateQAArbiterEvidenceRequests(qaMap, group, output.EvidenceRequests, outcomes)
	if err != nil {
		return QAArbiterGroup{}, err
	}
	base.Overrides, base.Issues, base.EvidenceRequests = output.Overrides, issues, requests
	return base, nil
}

// retainConfirmedQAArbiterIssueTheories canonicalizes the model output to the
// issue contract. Non-confirmed theories may be discussed by the arbiter, but
// cannot become repair-eligible issues. Confirmed coverage and uniqueness are
// still enforced by validateQAArbiterIssues after this normalization.
func retainConfirmedQAArbiterIssueTheories(issues []QAArbiterIssue, outcomes map[string]QATheoryOutcome) []QAArbiterIssue {
	retained := make([]QAArbiterIssue, 0, len(issues))
	for _, issue := range issues {
		confirmed := make([]string, 0, len(issue.TheoryIDs))
		for _, theoryID := range issue.TheoryIDs {
			if outcomes[theoryID] == QATheoryConfirmed {
				confirmed = append(confirmed, theoryID)
			}
		}
		issue.TheoryIDs = normalizeQAStrings(confirmed)
		if len(issue.TheoryIDs) > 0 {
			retained = append(retained, issue)
		}
	}
	return retained
}

// discardResolvedQAArbiterEvidenceRequests removes requests made redundant by
// the arbiter's decisions in the same output. A terminal outcome and a request
// for more evidence about that outcome are contradictory; the decision wins.
func discardResolvedQAArbiterEvidenceRequests(requests []QAArbiterEvidenceRequest, outcomes map[string]QATheoryOutcome) []QAArbiterEvidenceRequest {
	retained := make([]QAArbiterEvidenceRequest, 0, len(requests))
	for _, request := range requests {
		resolved := false
		for _, theoryID := range request.TheoryIDs {
			switch outcomes[theoryID] {
			case QATheoryConfirmed, QATheoryRefuted, QATheoryInvalid, QATheoryNotApplicable:
				resolved = true
			}
		}
		if !resolved {
			retained = append(retained, request)
		}
	}
	return retained
}

func normalizeQAArbiterReferences(output *qaArbiterOutput, allowed map[string]bool) {
	for i := range output.Overrides {
		output.Overrides[i].ReasonRefs = retainedQAArbiterReferences(output.Overrides[i].ReasonRefs, output.Overrides[i].TheoryIDs, allowed)
	}
	for i := range output.Issues {
		output.Issues[i].EvidenceRefs = retainedQAArbiterReferences(output.Issues[i].EvidenceRefs, output.Issues[i].TheoryIDs, allowed)
	}
}

func retainedQAArbiterReferences(current, theoryIDs []string, allowed map[string]bool) []string {
	retained := make([]string, 0, len(current)+len(theoryIDs))
	for _, id := range current {
		if allowed[id] {
			retained = append(retained, id)
		}
	}
	for _, id := range theoryIDs {
		if allowed[id] {
			retained = append(retained, id)
		}
	}
	return normalizeQAStrings(retained)
}

func validateQAArbiterEvidenceRequests(qaMap QAMap, group qaTheoryGroupPlan, requests []QAArbiterEvidenceRequest, outcomes map[string]QATheoryOutcome) ([]QAArbiterEvidenceRequest, error) {
	theories := make(map[string]QATheory, len(group.Theories))
	for _, theory := range group.Theories {
		theories[theory.ID] = theory
	}
	perShard := map[string]int{}
	seen := map[string]bool{}
	for i := range requests {
		request := &requests[i]
		if request.ArbiterGroupID != "" && request.ArbiterGroupID != group.ID {
			return nil, fmt.Errorf("arbiter evidence request claims another arbiter group")
		}
		request.ArbiterGroupID = group.ID
		request.TheoryIDs = normalizeQAStrings(request.TheoryIDs)
		request.OriginShardID = strings.TrimSpace(request.OriginShardID)
		request.Gap = strings.TrimSpace(request.Gap)
		request.RequestedEvidence = strings.TrimSpace(request.RequestedEvidence)
		request.RequiredObservation = strings.TrimSpace(request.RequiredObservation)
		request.ControlRequirement = strings.TrimSpace(request.ControlRequirement)
		request.Priority = strings.TrimSpace(request.Priority)
		if len(request.TheoryIDs) == 0 || !validQAIDKind(request.OriginShardID, "shard") || request.Gap == "" || request.RequestedEvidence == "" || request.RequiredObservation == "" || request.ControlRequirement == "" || request.Priority == "" {
			return nil, fmt.Errorf("arbiter evidence request is incomplete")
		}
		if request.Priority != "high" && request.Priority != "medium" && request.Priority != "low" {
			return nil, fmt.Errorf("arbiter evidence request priority %q is invalid", request.Priority)
		}
		for _, theoryID := range request.TheoryIDs {
			theory, ok := theories[theoryID]
			if !ok {
				return nil, fmt.Errorf("arbiter evidence request references unknown theory %q", theoryID)
			}
			if theory.ShardID != request.OriginShardID {
				return nil, fmt.Errorf("arbiter evidence request mixes or mismatches origin shards")
			}
			switch outcomes[theoryID] {
			case QATheoryConfirmed, QATheoryRefuted, QATheoryInvalid, QATheoryNotApplicable:
				return nil, fmt.Errorf("arbiter evidence request targets already sufficient evidence")
			}
		}
		identity, err := fingerprintQAValue(struct {
			Shard, Gap, Evidence, Observation, Control string
			Theories                                   []string
		}{request.OriginShardID, request.Gap, request.RequestedEvidence, request.RequiredObservation, request.ControlRequirement, request.TheoryIDs})
		if err != nil {
			return nil, err
		}
		if seen[identity] {
			return nil, fmt.Errorf("duplicate arbiter evidence request")
		}
		seen[identity] = true
		perShard[request.OriginShardID]++
		if perShard[request.OriginShardID] > qaMap.Budgets.EvidenceRoundsPerShard {
			return nil, fmt.Errorf("arbiter evidence requests exceed the shard round budget")
		}
		request.ID, err = NewQAV2ID("request", qaMap.Project, qaMap.Sprint, group.ID, identity)
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(requests, func(i, j int) bool { return requests[i].ID < requests[j].ID })
	return requests, nil
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
		repairDecodeErr := decodeStrictQAJSON(repaired.TerminalOutput, &output)
		if repairErr == nil && repairDecodeErr == nil {
			if issues, validationErr := validateQAReconcilerOutput(qaMap, provisional, output); validationErr == nil {
				return issues, record, nil
			} else {
				repairDecodeErr = validationErr
			}
		}
		decodeErr = errors.Join(decodeErr, repairErr, repairDecodeErr)
	}
	summary := "issue reconciliation agent did not produce valid output"
	if reason := safeError(decodeErr); reason != "" {
		summary += ": " + reason
	}
	return nil, record, NewQAError(QAErrorInvalidState, "reconcile QA issues", summary, errors.Join(runErr, decodeErr))
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
