package sprint

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type qaArbiterOutput struct {
	SchemaVersion int                 `json:"schema_version"`
	Overrides     []QAArbiterOverride `json:"overrides"`
}

func (s Service) arbitrateQA(ctx context.Context, qaMap QAMap, shards []QAShard, target string) QAArbitration {
	fallback := QAArbitration{SchemaVersion: QASchemaVersion, MapID: qaMap.ID, Fallback: true, Reason: "arbiter unavailable; investigator records retained"}
	if qaMap.Foundation == nil {
		return fallback
	}
	foundation, err := canonicalQAJSON(qaMap.Foundation)
	if err != nil {
		return fallback
	}
	theories := make([]QATheory, 0)
	for _, shard := range shards {
		theories = append(theories, shard.Theories...)
	}
	packet, err := canonicalQAJSON(struct {
		MapID    string     `json:"map_id"`
		Theories []QATheory `json:"theories"`
	}{qaMap.ID, theories})
	if err != nil {
		return fallback
	}
	prefix := `# Cross-shard QA arbiter

Review the frozen investigator theories against the wider QA foundation. You may confirm, refute, replace, merge, or split a theory's semantic conclusion. You cannot invent evidence, requirements, blocks, paths, or checks. You cannot authorize a patch or weaken a criterion. Return exactly one JSON object with schema_version 1 and overrides. Each override has theory_ids, action, outcome, replacement_claim, reason, reason_refs, and confidence. reason_refs must cite existing theory IDs or foundation block IDs. An empty overrides array is valid.

Frozen QA foundation:
` + string(foundation) + "\n\n<<< END STABLE QA ARBITER PREFIX >>>\n"
	prompt := prefix + "\nFrozen theory packet:\n" + string(packet) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		fallback.Reason = "arbiter prompt exceeded the frozen prompt budget"
		return fallback
	}
	settings, err := s.effectiveQASettings()
	if err != nil {
		return fallback
	}
	runtimeSettings := settings.RuntimeFor("challenger")
	provider, model := splitProviderModel(runtimeSettings.Model)
	fallback.Model = provider + "/" + model
	req := s.runtimeRequest(prompt, map[string]string{"project": qaMap.Project, "sprint": qaMap.Sprint, "stage": string(VerificationPhaseQA), "map": qaMap.ID, "role": "arbiter"})
	req.Provider, req.Model = provider, model
	req.Metadata["variant"], req.Metadata["reasoning_effort"] = runtimeSettings.Variant, runtimeSettings.Variant
	req.WorkDir, req.Timeout, req.Sandbox, req.Permissions = filepath.Clean(target), settings.Budgets.ShardTimeout, "read_only", "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = qaNoToolPolicy()
	req.Cache = pruntime.CacheDirective{Key: "qa-arbiter/" + qaMap.Foundation.Fingerprint + "/" + provider + "/" + model + "/" + runtimeSettings.Variant, BreakpointBytes: len(prefix), PrefixDigest: hashBytes([]byte(prefix)), Mode: "stable-prefix"}
	result, runErr := s.startQARuntime(ctx, qaMap, req)
	var output qaArbiterOutput
	decodeErr := decodeStrictQAJSON(result.TerminalOutput, &output)
	if runErr == nil && decodeErr == nil {
		if arbitration, validationErr := validateQAArbiterOutput(qaMap, theories, output, provider+"/"+model); validationErr == nil {
			return arbitration
		} else {
			decodeErr = validationErr
		}
	}
	if result.SessionID != "" && ctx.Err() == nil {
		repair := req
		repair.Prompt = "Your arbiter output was rejected: " + safeError(decodeErr) + ". Return only corrected JSON using existing theory and block references.\n"
		repair.SessionID, repair.SessionAction, repair.Cache = result.SessionID, "continue", pruntime.CacheDirective{}
		repaired, repairErr := s.startQARuntime(ctx, qaMap, repair)
		if repairErr == nil && decodeStrictQAJSON(repaired.TerminalOutput, &output) == nil {
			if arbitration, validationErr := validateQAArbiterOutput(qaMap, theories, output, provider+"/"+model); validationErr == nil {
				return arbitration
			}
		}
	}
	fallback.Reason = "arbiter output failed strict validation"
	return fallback
}

func validateQAArbiterOutput(qaMap QAMap, theories []QATheory, output qaArbiterOutput, model string) (QAArbitration, error) {
	if output.SchemaVersion != QASchemaVersion || len(output.Overrides) > qaMap.Budgets.FollowUpShards*qaMap.Budgets.TheoriesPerShard {
		return QAArbitration{}, fmt.Errorf("arbiter schema or override count is invalid")
	}
	refs := map[string]bool{}
	for _, theory := range theories {
		refs[theory.ID] = true
	}
	for _, block := range qaMap.Foundation.Blocks {
		refs[block.ID] = true
	}
	seenTheory := map[string]bool{}
	for i := range output.Overrides {
		override := &output.Overrides[i]
		override.TheoryIDs = normalizeQAStrings(override.TheoryIDs)
		override.ReasonRefs = normalizeQAStrings(override.ReasonRefs)
		if len(override.TheoryIDs) == 0 || len(override.ReasonRefs) == 0 || strings.TrimSpace(override.Reason) == "" || override.Confidence < 0 || override.Confidence > 1 {
			return QAArbitration{}, fmt.Errorf("arbiter override is incomplete")
		}
		switch override.Action {
		case QAArbiterConfirm, QAArbiterRefute, QAArbiterReplace, QAArbiterMerge, QAArbiterSplit:
		default:
			return QAArbitration{}, fmt.Errorf("arbiter action %q is invalid", override.Action)
		}
		if override.Outcome != QATheoryConfirmed && override.Outcome != QATheoryRefuted && override.Outcome != QATheoryInconclusive && override.Outcome != QATheoryCrossShard {
			return QAArbitration{}, fmt.Errorf("arbiter outcome %q is invalid", override.Outcome)
		}
		for _, id := range override.TheoryIDs {
			if !refs[id] || seenTheory[id] {
				return QAArbitration{}, fmt.Errorf("arbiter theory reference is unknown or superseded twice")
			}
			seenTheory[id] = true
		}
		for _, id := range override.ReasonRefs {
			if !refs[id] {
				return QAArbitration{}, fmt.Errorf("arbiter reason reference %q is unknown", id)
			}
		}
		identity, err := fingerprintQAValue(struct {
			Map      string
			Theories []string
			Action   QAArbiterAction
			Outcome  QATheoryOutcome
			Reason   string
			Refs     []string
		}{qaMap.ID, override.TheoryIDs, override.Action, override.Outcome, override.Reason, override.ReasonRefs})
		if err != nil {
			return QAArbitration{}, err
		}
		override.ID = QAIDScope + "-override-" + identity[:24]
	}
	return QAArbitration{SchemaVersion: QASchemaVersion, MapID: qaMap.ID, Model: model, Overrides: output.Overrides}, nil
}

func applyQAArbitration(shards []QAShard, arbitration QAArbitration) []QAShard {
	result := append([]QAShard(nil), shards...)
	outcomes := map[string]QATheoryOutcome{}
	reasons := map[string]string{}
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
