package sprint

import (
	"context"
	"encoding/json"
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

func (s Service) refineQAMapSemantically(ctx context.Context, qaMap QAMap, target string) QAMap {
	if qaMap.Foundation == nil || len(qaMap.Foundation.Blocks) == 0 {
		return qaMap
	}
	foundation, err := canonicalQAJSON(qaMap.Foundation)
	if err != nil {
		return qaMap
	}
	candidate, err := canonicalQAJSON(struct {
		ChangedPaths []string  `json:"changed_paths"`
		Fallback     []QAShard `json:"deterministic_fallback"`
		Budgets      QABudgets `json:"budgets"`
	}{qaMap.Coverage.ChangedPaths, qaMap.Shards, qaMap.Budgets})
	if err != nil {
		return qaMap
	}
	prefix := `# QA semantic mapper

Propose behavioral QA shards from the frozen foundation. Use no tools and return exactly one JSON object. The object has schema_version 1 and shards. Each shard has kind, title, changed_paths, context_paths, overlap_paths, boundary_reason, behavioral_concerns, expectation_refs, and context_block_ids. Primary shards must assign every changed path exactly once. Boundary shards may overlap paths but cannot own them. Cite only foundation block IDs and exact expectation IDs. Counts equal to a maximum are valid. Do not invent paths or requirements.

Frozen QA foundation:
` + string(foundation) + "\n\n<<< END STABLE QA MAPPER PREFIX >>>\n"
	prompt := prefix + "\nMap input:\n" + string(candidate) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		qaMap.Mapper = &QAMapperRecord{Executor: "deterministic", Fallback: true, Reason: "semantic mapper prompt exceeded the frozen prompt budget", PromptBytes: len(prompt), PrefixBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix))}
		return qaMap
	}
	settings, err := s.effectiveQASettings()
	if err != nil {
		return qaMap
	}
	runtimeSettings := settings.RuntimeFor("challenger")
	provider, model := splitProviderModel(runtimeSettings.Model)
	req := s.runtimeRequest(prompt, map[string]string{"project": qaMap.Project, "sprint": qaMap.Sprint, "stage": string(VerificationPhaseQA), "map": qaMap.ID, "role": "semantic-mapper"})
	req.Provider, req.Model = provider, model
	req.Metadata["variant"], req.Metadata["reasoning_effort"] = runtimeSettings.Variant, runtimeSettings.Variant
	req.WorkDir, req.Timeout, req.Sandbox, req.Permissions = filepath.Clean(target), settings.Budgets.ShardTimeout, "read_only", "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = qaNoToolPolicy()
	req.Cache = pruntime.CacheDirective{Key: "qa-mapper/" + qaMap.Foundation.Fingerprint + "/" + provider + "/" + model + "/" + runtimeSettings.Variant, BreakpointBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix)), Mode: "stable-prefix"}
	result, runErr := s.startQARuntime(ctx, qaMap, req)
	var output qaSemanticMapperOutput
	decodeErr := decodeStrictQAJSON(result.TerminalOutput, &output)
	if runErr == nil && decodeErr == nil {
		if mapped, mapErr := applyQASemanticMap(qaMap, output); mapErr == nil {
			mapped.Mapper = &QAMapperRecord{Executor: "model", Model: provider + "/" + model, PromptBytes: len(prompt), PrefixBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix))}
			return mapped
		} else {
			decodeErr = mapErr
		}
	}
	if result.SessionID != "" && ctx.Err() == nil {
		repair := req
		repair.Prompt = "Your semantic map was rejected: " + safeError(decodeErr) + ". Return only a corrected schema_version 1 JSON object. Reuse the frozen foundation and do not repeat analysis.\n"
		repair.SessionID, repair.SessionAction, repair.Cache = result.SessionID, "continue", pruntime.CacheDirective{}
		repaired, repairErr := s.startQARuntime(ctx, qaMap, repair)
		if repairErr == nil && decodeStrictQAJSON(repaired.TerminalOutput, &output) == nil {
			if mapped, mapErr := applyQASemanticMap(qaMap, output); mapErr == nil {
				mapped.Mapper = &QAMapperRecord{Executor: "model", Model: provider + "/" + model, PromptBytes: len(prompt) + len(repair.Prompt), PrefixBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix))}
				return mapped
			}
		}
	}
	qaMap.Mapper = &QAMapperRecord{Executor: "deterministic", Model: provider + "/" + model, Fallback: true, Reason: "semantic mapper output failed strict validation", PromptBytes: len(prompt), PrefixBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix))}
	return qaMap
}

func applyQASemanticMap(qaMap QAMap, output qaSemanticMapperOutput) (QAMap, error) {
	if output.SchemaVersion != QASchemaVersion || len(output.Shards) == 0 || len(output.Shards) > qaMap.Budgets.TotalShards {
		return QAMap{}, fmt.Errorf("semantic map schema or shard count is invalid")
	}
	paths := stringSet(qaMap.Coverage.ChangedPaths)
	availableExpectations := map[string]bool{}
	availableBlocks := map[string]bool{}
	blockExpectations := map[string]map[string]bool{}
	availablePaths := map[string]bool{}
	for _, block := range qaMap.Foundation.Blocks {
		availableBlocks[block.ID] = true
		blockExpectations[block.ID] = stringSet(block.ExpectationRefs)
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
		proposal.ContextPaths = normalizeQAStrings(proposal.ContextPaths)
		proposal.OverlapPaths = normalizeQAStrings(proposal.OverlapPaths)
		proposal.BehavioralConcerns = normalizeQAStrings(proposal.BehavioralConcerns)
		proposal.ExpectationRefs = normalizeQAStrings(proposal.ExpectationRefs)
		proposal.ContextBlockIDs = normalizeQAStrings(proposal.ContextBlockIDs)
		if strings.TrimSpace(proposal.Title) == "" || len(proposal.BehavioralConcerns) == 0 || len(proposal.BehavioralConcerns) > qaMap.Budgets.BehavioralConcernsPerShard || len(proposal.ExpectationRefs) == 0 || len(proposal.ContextPaths) > qaMap.Budgets.ContextPathsPerShard {
			return QAMap{}, fmt.Errorf("semantic shard is incomplete or over budget")
		}
		for _, path := range append(append([]string(nil), proposal.ContextPaths...), proposal.OverlapPaths...) {
			if validateQAPath(path) != nil || !availablePaths[path] {
				return QAMap{}, fmt.Errorf("semantic shard invented context path %q", path)
			}
		}
		for _, ref := range proposal.ExpectationRefs {
			if !availableExpectations[ref] {
				return QAMap{}, fmt.Errorf("semantic shard invented expectation %q", ref)
			}
			cited := false
			for _, id := range proposal.ContextBlockIDs {
				if blockExpectations[id][ref] {
					cited = true
					break
				}
			}
			if !cited {
				return QAMap{}, fmt.Errorf("semantic shard did not cite an exact block for expectation %q", ref)
			}
		}
		for _, id := range proposal.ContextBlockIDs {
			if !availableBlocks[id] {
				return QAMap{}, fmt.Errorf("semantic shard invented context block %q", id)
			}
		}
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

func qaNoToolPolicy() pruntime.PermissionPolicy {
	return pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "deny", "list": "deny", "search": "deny", "glob": "deny", "write": "deny", "edit": "deny", "patch": "deny", "bash": "deny", "shell": "deny"}}
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
		return s.runtime.StartRun(ctx, req)
	}
	return s.startSprintRuntime(ctx, sp, PlanningStage(VerificationPhaseQA), req)
}
