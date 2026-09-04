package sprint

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type qaSemanticShardProposal struct {
	Kind               QAShardKind `json:"kind"`
	Title              string      `json:"title"`
	ChangedPaths       []string    `json:"changed_paths"`
	ContextPaths       []string    `json:"context_paths"`
	OverlapPaths       []string    `json:"overlap_paths"`
	BoundaryReason     string      `json:"boundary_reason"`
	BehavioralConcerns []string    `json:"behavioral_concerns"`
	ExpectationRefs    []string    `json:"expectation_refs"`
	ContextBlockIDs    []string    `json:"context_block_ids"`
}

type qaSemanticMapperOutput struct {
	SchemaVersion int                       `json:"schema_version"`
	Shards        []qaSemanticShardProposal `json:"shards"`
}

const qaSemanticMapperOutputContract = `Return only schema_version and shards. Each shard may contain only kind, title, changed_paths, context_paths, overlap_paths, boundary_reason, behavioral_concerns, expectation_refs, and context_block_ids. Omit id, fingerprint, status, attempts, checks, timestamps, and every other product-owned or fallback field; UltraPlan assigns identity and operational state.`

func (s Service) refineQAMapSemantically(ctx context.Context, qaMap QAMap, target string) (QAMap, error) {
	if qaMap.Foundation == nil || len(qaMap.Foundation.Blocks) == 0 {
		return QAMap{}, NewQAError(QAErrorInvalidState, "semantic map", "frozen QA foundation is unavailable", nil)
	}
	foundation, err := canonicalQAJSON(qaMap.Foundation)
	if err != nil {
		return QAMap{}, NewQAError(QAErrorInvalidState, "semantic map", "cannot encode frozen QA foundation", err)
	}
	candidate, err := canonicalQAJSON(struct {
		ChangedPaths []string  `json:"changed_paths"`
		Fallback     []QAShard `json:"deterministic_fallback"`
		Budgets      QABudgets `json:"budgets"`
	}{qaMap.Coverage.ChangedPaths, qaMap.Shards, qaMap.Budgets})
	if err != nil {
		return QAMap{}, NewQAError(QAErrorInvalidState, "semantic map", "cannot encode semantic mapper input", err)
	}
	prefix := `# QA semantic mapper

Propose behavioral QA shards from the frozen foundation and return exactly one JSON object. The supplied foundation should normally be sufficient. You may use bounded read-only repository tools when they are needed to resolve a material gap. ` + qaSemanticMapperOutputContract + ` Primary shards must assign every changed path exactly once. Boundary shards may overlap paths but cannot own them. Cite only foundation block IDs and exact expectation IDs. Counts equal to a maximum are valid. Do not invent paths or requirements.

Frozen QA foundation:
` + string(foundation) + "\n\n<<< END STABLE QA MAPPER PREFIX >>>\n"
	prompt := prefix + "\nMap input:\n" + string(candidate) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		return QAMap{}, NewQAError(QAErrorBudgetExhausted, "semantic map", "semantic mapper prompt exceeded the frozen prompt budget", nil)
	}
	settings, err := s.effectiveQASettings()
	if err != nil {
		return QAMap{}, err
	}
	runtimeSettings := settings.RuntimeFor("mapper")
	provider, model := splitProviderModel(runtimeSettings.Model)
	req := s.runtimeRequest(prompt, map[string]string{"project": qaMap.Project, "sprint": qaMap.Sprint, "stage": string(VerificationPhaseQA), "map": qaMap.ID, "role": "semantic-mapper"})
	req.Metadata["operation"] = "qa-map"
	req.Metadata["task"] = qaMap.ID
	req.Metadata["qa_attempt"] = qaMap.SemanticAttemptID
	req.Provider, req.Model = provider, model
	req.Metadata["variant"], req.Metadata["reasoning_effort"] = runtimeSettings.Variant, runtimeSettings.Variant
	req.WorkDir, req.Timeout, req.Sandbox, req.Permissions = filepath.Clean(target), settings.Budgets.ShardTimeout, "read_only", "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = qaReadOnlyToolPolicy()
	req.Cache = pruntime.CacheDirective{Key: "qa-mapper/" + qaMap.Foundation.Fingerprint + "/" + provider + "/" + model + "/" + runtimeSettings.Variant, BreakpointBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix)), Mode: "stable-prefix"}
	result, runErr := s.startQARuntime(ctx, qaMap, req)
	var output qaSemanticMapperOutput
	decodeErr := decodeStrictQAJSON(result.TerminalOutput, &output)
	if runErr == nil && decodeErr == nil {
		if mapped, mapErr := applyQASemanticMap(qaMap, output); mapErr == nil {
			mapped.Mapper = &QAMapperRecord{Executor: "model", Model: provider + "/" + model, PromptBytes: len(prompt), PrefixBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix))}
			return mapped, nil
		} else {
			decodeErr = mapErr
		}
	}
	if result.SessionID != "" && ctx.Err() == nil {
		repair := req
		repair.Prompt = "Your semantic map was rejected: " + safeError(decodeErr) + ". Return only a corrected schema_version 1 JSON object. " + qaSemanticMapperOutputContract + " Reuse the frozen foundation and do not repeat analysis.\n"
		repair.SessionID, repair.SessionAction, repair.Cache = result.SessionID, "continue", pruntime.CacheDirective{}
		repair.Metadata["repair_of"] = result.RunID
		repaired, repairErr := s.startQARuntime(ctx, qaMap, repair)
		repairDecodeErr := decodeStrictQAJSON(repaired.TerminalOutput, &output)
		if repairErr == nil && repairDecodeErr == nil {
			if mapped, mapErr := applyQASemanticMap(qaMap, output); mapErr == nil {
				mapped.Mapper = &QAMapperRecord{Executor: "model", Model: provider + "/" + model, PromptBytes: len(prompt) + len(repair.Prompt), PrefixBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix))}
				return mapped, nil
			} else {
				repairDecodeErr = mapErr
			}
		}
		decodeErr = errors.Join(decodeErr, repairErr, repairDecodeErr)
	}
	failure := errors.Join(runErr, decodeErr)
	if failure == nil {
		failure = errors.New("semantic mapper output failed strict validation")
	}
	summary := "semantic mapper did not produce a valid complete map"
	if reason := safeError(failure); reason != "" {
		summary += ": " + reason
	}
	return QAMap{}, NewQAError(QAErrorInvalidState, "semantic map", summary, failure)
}

func applyQASemanticMap(qaMap QAMap, output qaSemanticMapperOutput) (QAMap, error) {
	if output.SchemaVersion != QASchemaVersion || len(output.Shards) == 0 || len(output.Shards) > qaMap.Budgets.TotalShards {
		return QAMap{}, fmt.Errorf("semantic map schema or shard count is invalid")
	}
	paths := stringSet(qaMap.Coverage.ChangedPaths)
	availableExpectations := map[string]bool{}
	availableBlocks := map[string]bool{}
	availablePaths := map[string]bool{}
	for _, block := range qaMap.Foundation.Blocks {
		availableBlocks[block.ID] = true
		if block.Kind == "source" && block.Path != "" {
			availablePaths[block.Path] = true
		}
		for _, ref := range block.ExpectationRefs {
			availableExpectations[ref] = true
		}
	}
	for _, fallback := range qaMap.Shards {
		for _, path := range append(append([]string(nil), fallback.ChangedPaths...), fallback.ContextPaths...) {
			availablePaths[path] = true
		}
	}
	owners := map[string]string{}
	shards := make([]QAShard, 0, len(output.Shards))
	primaryCount, boundaryCount := 0, 0
	for _, proposal := range output.Shards {
		proposal.ChangedPaths = normalizeQAStrings(proposal.ChangedPaths)
		proposal.ContextPaths = retainQASemanticReferences(proposal.ContextPaths, availablePaths)
		proposal.OverlapPaths = retainQASemanticReferences(proposal.OverlapPaths, availablePaths)
		proposal.BehavioralConcerns = normalizeQAStrings(proposal.BehavioralConcerns)
		proposal.ExpectationRefs = retainQASemanticReferences(proposal.ExpectationRefs, availableExpectations)
		if len(proposal.ExpectationRefs) == 0 {
			proposal.ExpectationRefs = fallbackQASemanticExpectations(qaMap.Shards, proposal)
		}
		proposal.ContextBlockIDs = retainQASemanticReferences(proposal.ContextBlockIDs, availableBlocks)
		if strings.TrimSpace(proposal.Title) == "" || len(proposal.BehavioralConcerns) == 0 || len(proposal.BehavioralConcerns) > qaMap.Budgets.BehavioralConcernsPerShard || len(proposal.ExpectationRefs) == 0 || len(proposal.ContextPaths) > qaMap.Budgets.ContextPathsPerShard {
			return QAMap{}, fmt.Errorf("semantic shard is incomplete or over budget")
		}
		for _, path := range append(append([]string(nil), proposal.ContextPaths...), proposal.OverlapPaths...) {
			if validateQAPath(path) != nil {
				return QAMap{}, fmt.Errorf("semantic shard context path is unsafe %q", path)
			}
		}
		for _, ref := range proposal.ExpectationRefs {
			if !availableExpectations[ref] {
				return QAMap{}, fmt.Errorf("semantic shard invented expectation %q", ref)
			}
		}
		for _, id := range proposal.ContextBlockIDs {
			if !availableBlocks[id] {
				return QAMap{}, fmt.Errorf("semantic shard invented context block %q", id)
			}
		}
		proposal.ContextBlockIDs = qaCompleteExpectationProjection(qaMap.Foundation, proposal)
		identity := QAShardIdentity{Kind: proposal.Kind, ChangedPaths: proposal.ChangedPaths, ContextPaths: proposal.ContextPaths, BehavioralConcerns: proposal.BehavioralConcerns, ExpectationRefs: proposal.ExpectationRefs}
		id, err := NewQAShardID(qaMap.Project, qaMap.Sprint, qaMap.ID, identity)
		if err != nil {
			return QAMap{}, err
		}
		shard := QAShard{SchemaVersion: QASchemaVersion, ID: id, AttemptID: qaMap.SemanticAttemptID, Kind: proposal.Kind, Title: strings.TrimSpace(proposal.Title), ChangedPaths: proposal.ChangedPaths, ContextPaths: proposal.ContextPaths, OverlapPaths: proposal.OverlapPaths, BoundaryReason: strings.TrimSpace(proposal.BoundaryReason), BehavioralConcerns: proposal.BehavioralConcerns, ExpectationRefs: proposal.ExpectationRefs, ContextBlockIDs: proposal.ContextBlockIDs, RiskTags: qaTagsForPaths(qaRiskTags(qaMap.Coverage.ChangedPaths), append(proposal.ChangedPaths, proposal.OverlapPaths...)), ApprovedChecks: append([]QAApprovedCheckRef(nil), qaMap.Foundation.ApprovedChecks...), Phase: QAPhaseMapped}
		switch proposal.Kind {
		case QAShardPrimary:
			primaryCount++
			if len(proposal.ChangedPaths) == 0 || len(proposal.ChangedPaths) > qaMap.Budgets.ChangedPathsPerShard {
				return QAMap{}, fmt.Errorf("semantic primary shard path count is invalid")
			}
			for _, path := range proposal.ChangedPaths {
				if !paths[path] || owners[path] != "" {
					return QAMap{}, fmt.Errorf("semantic primary ownership is incomplete or duplicated")
				}
				owners[path] = id
			}
		case QAShardBoundary:
			boundaryCount++
			if len(proposal.OverlapPaths) == 0 || strings.TrimSpace(proposal.BoundaryReason) == "" {
				return QAMap{}, fmt.Errorf("semantic boundary shard is incomplete")
			}
		default:
			return QAMap{}, fmt.Errorf("semantic mapper may return only primary and boundary shards")
		}
		shards = append(shards, shard)
	}
	if primaryCount > qaMap.Budgets.PrimaryShards || boundaryCount > qaMap.Budgets.BoundaryShards || len(owners) != len(paths) {
		return QAMap{}, fmt.Errorf("semantic map does not satisfy coverage budgets")
	}
	for path := range paths {
		if owners[path] == "" {
			return QAMap{}, fmt.Errorf("semantic map omitted changed path %q", path)
		}
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].ID < shards[j].ID })
	qaMap.Shards = shards
	qaMap.Coverage.PrimaryOwners = owners
	qaMap.Coverage.BoundaryOverlaps = map[string][]string{}
	for _, shard := range shards {
		if shard.Kind == QAShardBoundary {
			qaMap.Coverage.BoundaryOverlaps[shard.ID] = append([]string(nil), shard.OverlapPaths...)
		}
	}
	if len(qaMap.Coverage.BoundaryOverlaps) == 0 {
		qaMap.Coverage.BoundaryOverlaps = nil
	}
	return qaMap, ValidateQAMap(qaMap)
}

func retainQASemanticReferences(values []string, allowed map[string]bool) []string {
	retained := make([]string, 0, len(values))
	for _, value := range values {
		if allowed[value] {
			retained = append(retained, value)
		}
	}
	return normalizeQAStrings(retained)
}

func fallbackQASemanticExpectations(fallback []QAShard, proposal qaSemanticShardProposal) []string {
	paths := stringSet(append(append(append([]string(nil), proposal.ChangedPaths...), proposal.ContextPaths...), proposal.OverlapPaths...))
	var refs []string
	for _, shard := range fallback {
		overlaps := false
		for _, path := range append(append([]string(nil), shard.ChangedPaths...), shard.ContextPaths...) {
			if paths[path] {
				overlaps = true
				break
			}
		}
		if overlaps {
			refs = append(refs, shard.ExpectationRefs...)
		}
	}
	return normalizeQAStrings(refs)
}

func qaCompleteExpectationProjection(foundation *QAFoundation, proposal qaSemanticShardProposal) []string {
	selected := stringSet(proposal.ContextBlockIDs)
	query := strings.ToLower(strings.Join(append(append([]string{proposal.Title}, proposal.BehavioralConcerns...), append(proposal.ChangedPaths, proposal.ContextPaths...)...), " "))
	queryTerms := qaProjectionTerms(query)
	for _, ref := range proposal.ExpectationRefs {
		covered := false
		for _, block := range foundation.Blocks {
			if selected[block.ID] && containsQAString(block.ExpectationRefs, ref) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		bestID, bestScore := "", -1
		for _, block := range foundation.Blocks {
			if !containsQAString(block.ExpectationRefs, ref) {
				continue
			}
			score := 0
			for term := range queryTerms {
				if strings.Contains(strings.ToLower(block.Content), term) {
					score++
				}
			}
			if score > bestScore || score == bestScore && block.ID < bestID {
				bestID, bestScore = block.ID, score
			}
		}
		if bestID != "" {
			selected[bestID] = true
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func qaProjectionTerms(value string) map[string]bool {
	terms := map[string]bool{}
	for _, term := range strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	}) {
		if len(term) >= 4 {
			terms[term] = true
		}
	}
	return terms
}

func decodeStrictQAJSON(content string, target any) error {
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(content)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("model output contains more than one JSON value")
		}
		return err
	}
	return nil
}

func qaReadOnlyToolPolicy() pruntime.PermissionPolicy {
	return pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow", "glob": "allow", "write": "deny", "edit": "deny", "patch": "deny", "bash": "deny", "shell": "deny"}}
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[value] = true
	}
	return result
}

func (s Service) startQARuntime(ctx context.Context, qaMap QAMap, req pruntime.Request) (pruntime.Result, error) {
	sp, err := s.resolveMutationSprint(qaMap.Project, qaMap.Sprint)
	if err != nil {
		return pruntime.Result{}, NewQAError(QAErrorInvalidState, "start QA runtime", "cannot resolve the sprint telemetry ledger before dispatch", err)
	}
	return s.startSprintRuntime(ctx, sp, PlanningStage(VerificationPhaseQA), req)
}
