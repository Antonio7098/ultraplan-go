package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

type QAMapInput struct {
	Project                   string
	Sprint                    string
	ChangedPaths              []string
	ContextPaths              map[string][]string
	ExpectationRefs           []string
	RiskTags                  map[string][]string
	InputRefs                 []QAArtifactRef
	GovernedInputFingerprint  string
	ImplementationFingerprint string
	ReviewFingerprint         string
	PolicyFingerprint         string
	CheckCatalogFingerprint   string
	Target                    QATargetIdentity
	Settings                  QASettings
	ApprovedChecks            []QAApprovedCheckRef
	Foundation                *QAFoundation
}

type QAMapResult struct {
	Map             QAMap  `json:"map"`
	NormalizedBytes []byte `json:"-"`
	DryRun          bool   `json:"dry_run"`
}

// QAMap builds a deterministic map without accepting a run, constructing a
// runtime, or writing product state.
func (s Service) QAMap(projectRef, sprintRef string) (QAMapResult, error) {
	settings, err := s.effectiveQASettings()
	if err != nil {
		return QAMapResult{}, NewQAError(QAErrorInvalidState, "map", "effective QA settings are invalid", err)
	}
	manifest, findings, err := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	if err != nil {
		return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", "cannot prepare governed Conformance Review inputs", err)
	}
	if len(findings) > 0 {
		return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", fmt.Sprintf("governed inputs have %d validation findings", len(findings)), nil)
	}
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return QAMapResult{}, err
	}
	flow, err := LoadFlowState(s.root, sp)
	if err != nil {
		return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", "flow state is unavailable", err)
	}
	if flow.Review == nil || flow.Review.Stale || strings.TrimSpace(flow.Review.Fingerprint) == "" {
		return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", "a current Conformance Review record is required", nil)
	}
	switch flow.Review.Status {
	case ReviewCompleted, ReviewBlocked:
	default:
		return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", "Conformance Review is not terminal", nil)
	}
	changed := normalizeQAStrings(manifest.ChangedPaths)
	if len(changed) == 0 || len(changed) == 1 && strings.HasPrefix(changed[0], "(") {
		return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", "execute evidence does not enumerate changed paths", nil)
	}
	implementationFingerprint, err := targetIdentity(manifest.Target)
	if err != nil {
		return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", "target identity is unavailable", err)
	}
	inputRefs := make([]QAArtifactRef, 0, len(manifest.Inputs)+1)
	for _, input := range manifest.Inputs {
		if input.Hash == "" || input.Hash == "missing" {
			return QAMapResult{}, NewQAError(QAErrorStaleInput, "map", "governed input is missing: "+input.Path, nil)
		}
		inputRefs = append(inputRefs, QAArtifactRef{Path: filepath.ToSlash(input.Path), Digest: input.Hash})
	}
	if flow.Review.ArtifactDigest != "" {
		inputRefs = append(inputRefs, QAArtifactRef{Path: ArtifactRelPath(sp, StageReview), Digest: flow.Review.ArtifactDigest})
	}
	governedFingerprint, err := fingerprintQAValue(inputRefs)
	if err != nil {
		return QAMapResult{}, err
	}
	policyFingerprint, err := fingerprintQAValue(settings)
	if err != nil {
		return QAMapResult{}, err
	}
	context := qaAdjacentContext(manifest.Target, changed, settings.Budgets.ContextPathsPerShard)
	for _, paths := range context {
		for _, rel := range paths {
			data, readErr := os.ReadFile(filepath.Join(manifest.Target, filepath.FromSlash(rel)))
			if readErr == nil {
				manifest.Contents["target/"+rel] = string(data)
			}
		}
	}
	expectations := make([]string, 0, len(manifest.Coverage))
	for _, coverage := range manifest.Coverage {
		expectations = append(expectations, coverage.ID)
	}
	if len(expectations) == 0 {
		expectations = []string{"governed-plan"}
	}
	gitHead, gitIndex, gitWorktree := qaGitIdentity(manifest.Target)
	checks, err := ApprovedQAChecks(manifest.Target, changed, settings.Budgets)
	if err != nil {
		return QAMapResult{}, NewQAError(QAErrorInvalidState, "map", "approved check catalog is invalid", err)
	}
	checkRefs := make([]QAApprovedCheckRef, 0, len(checks))
	for _, check := range checks {
		checkRefs = append(checkRefs, QAApprovedCheckRef{ID: check.ID, Fingerprint: check.Fingerprint})
	}
	checkCatalogFingerprint, err := fingerprintQAValue(checks)
	if err != nil {
		return QAMapResult{}, err
	}
	target := QATargetIdentity{Fingerprint: implementationFingerprint, Scope: "current_checkout", GitHead: gitHead, GitIndex: gitIndex, GitWorktree: gitWorktree}
	addQAWorkspaceProvenance(&target, sp, manifest.Target)
	reviewArtifact, _ := s.store.ReadArtifact(sp, StageReview)
	foundation, err := BuildQAFoundation(manifest, governedFingerprint, implementationFingerprint, flow.Review.Fingerprint, checkRefs, settings.Budgets, reviewArtifact)
	if err != nil {
		return QAMapResult{}, NewQAError(QAErrorInvalidState, "map", "cannot build the frozen QA foundation", err)
	}
	input := QAMapInput{
		Project: sp.Project, Sprint: sp.Slug, ChangedPaths: changed, ContextPaths: context,
		ExpectationRefs: expectations, RiskTags: qaRiskTags(changed), InputRefs: inputRefs,
		GovernedInputFingerprint: governedFingerprint, ImplementationFingerprint: implementationFingerprint,
		ReviewFingerprint: flow.Review.Fingerprint, PolicyFingerprint: policyFingerprint,
		CheckCatalogFingerprint: checkCatalogFingerprint,
		Target:                  target,
		Settings:                settings,
		ApprovedChecks:          checkRefs,
		Foundation:              &foundation,
	}
	qaMap, err := BuildQAMap(input)
	if err != nil {
		return QAMapResult{}, err
	}
	if err := s.prepareQAShardPacks(&qaMap); err != nil {
		return QAMapResult{}, err
	}
	data, err := NormalizedQAMapBytes(qaMap)
	if err != nil {
		return QAMapResult{}, err
	}
	return QAMapResult{Map: qaMap, NormalizedBytes: data, DryRun: true}, nil
}

func (s Service) prepareQAShardPacks(qaMap *QAMap) error {
	for i := range qaMap.Shards {
		prompt, err := s.RenderQAInvestigatorPrompt(*qaMap, qaMap.Shards[i])
		if err != nil {
			return NewQAError(QAErrorBudgetExhausted, "map", "prepared investigator context exceeds the prompt budget", err)
		}
		qaMap.Shards[i].PreparedPromptBytes = len(prompt)
		marker := strings.Index(prompt, "<<< END STABLE QA INVESTIGATOR PREFIX >>>")
		if marker >= 0 {
			qaMap.Shards[i].PreparedPrefixBytes = marker + len("<<< END STABLE QA INVESTIGATOR PREFIX >>>\n")
		}
		qaMap.Shards[i].PackComplete = qaShardPackComplete(*qaMap, qaMap.Shards[i])
	}
	return nil
}

func addQAWorkspaceProvenance(target *QATargetIdentity, sp Sprint, targetPath string) {
	record, err := loadSprintWorkspace(sp)
	if err != nil || filepath.Clean(record.Path) != filepath.Clean(targetPath) {
		return
	}
	target.WorkspaceBranch = record.Branch
	target.WorkspaceBaseline = record.Baseline
	if record.Baseline == "" || target.GitHead == "" {
		return
	}
	if record.Baseline == target.GitHead {
		target.BaselineRelation = "at_baseline"
		return
	}
	if err := exec.Command("git", "-C", targetPath, "merge-base", "--is-ancestor", record.Baseline, target.GitHead).Run(); err != nil {
		target.BaselineRelation = "diverged"
		return
	}
	output, err := exec.Command("git", "-C", targetPath, "rev-list", "--count", record.Baseline+".."+target.GitHead).Output()
	if err != nil {
		target.BaselineRelation = "ahead_of_baseline"
		return
	}
	target.BaselineRelation = "ahead_of_baseline"
	_, _ = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &target.CommitsSinceBase)
}

func BuildQAMap(input QAMapInput) (QAMap, error) {
	if !safeQAName(input.Project) || !safeQAName(input.Sprint) {
		return QAMap{}, NewQAError(QAErrorInvalidState, "map", "invalid project or sprint identity", nil)
	}
	if err := ValidateQASettings(input.Settings); err != nil {
		return QAMap{}, NewQAError(QAErrorInvalidState, "map", err.Error(), err)
	}
	for name, value := range map[string]string{"governed input": input.GovernedInputFingerprint, "implementation": input.ImplementationFingerprint, "review": input.ReviewFingerprint, "policy": input.PolicyFingerprint, "check catalog": input.CheckCatalogFingerprint, "target": input.Target.Fingerprint} {
		if !validFingerprint(value) {
			return QAMap{}, NewQAError(QAErrorInvalidState, "map", "invalid "+name+" fingerprint", nil)
		}
	}
	paths := normalizeQAStrings(input.ChangedPaths)
	if len(paths) == 0 {
		return QAMap{}, NewQAError(QAErrorStaleInput, "map", "no changed paths were supplied", nil)
	}
	if len(paths) > input.Settings.Budgets.ChangedPaths {
		return QAMap{}, NewQAError(QAErrorBudgetExhausted, "map", "changed path limit exceeded", nil)
	}
	for _, path := range paths {
		if err := validateQAPath(path); err != nil {
			return QAMap{}, NewQAError(QAErrorInvalidState, "map", err.Error(), err)
		}
	}
	identity := QASemanticIdentity{GovernedInputFingerprint: input.GovernedInputFingerprint, ImplementationFingerprint: input.ImplementationFingerprint, ReviewFingerprint: input.ReviewFingerprint, PolicyFingerprint: input.PolicyFingerprint, ChangedPaths: paths}
	attemptID, err := NewQASemanticAttemptID(input.Project, input.Sprint, identity)
	if err != nil {
		return QAMap{}, err
	}
	mapID, err := NewQAMapID(input.Project, input.Sprint, attemptID, identity)
	if err != nil {
		return QAMap{}, err
	}
	type group struct {
		key     string
		unknown bool
		paths   []string
	}
	groupsByKey := map[string]*group{}
	for _, path := range paths {
		key, known := qaBehaviorGroup(path)
		if !known {
			key = "unknown/" + path
		}
		g := groupsByKey[key]
		if g == nil {
			g = &group{key: key, unknown: !known}
			groupsByKey[key] = g
		}
		g.paths = append(g.paths, path)
	}
	keys := make([]string, 0, len(groupsByKey))
	for key := range groupsByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var shards []QAShard
	owners := map[string]string{}
	for _, key := range keys {
		g := groupsByKey[key]
		for start := 0; start < len(g.paths); start += input.Settings.Budgets.ChangedPathsPerShard {
			end := start + input.Settings.Budgets.ChangedPathsPerShard
			if end > len(g.paths) {
				end = len(g.paths)
			}
			chunk := append([]string(nil), g.paths[start:end]...)
			concerns := []string{"changed behavior in " + key}
			phase := QAPhaseMapped
			var blocker *QABlocker
			if g.unknown {
				phase = QAPhaseBlocked
				blocker = &QABlocker{Category: QAErrorInvalidState, Scope: "shard", Summary: "unclassified changed path", NextAction: "Add a deterministic path classifier before investigation."}
				concerns = []string{"unclassified changed path"}
			}
			context := qaContextForPaths(input.ContextPaths, chunk, input.Settings.Budgets.ContextPathsPerShard)
			riskTags := qaTagsForPaths(input.RiskTags, chunk)
			shardIdentity := QAShardIdentity{Kind: QAShardPrimary, ChangedPaths: chunk, ContextPaths: context, BehavioralConcerns: concerns, ExpectationRefs: normalizeQAStrings(input.ExpectationRefs)}
			shardID, idErr := NewQAShardID(input.Project, input.Sprint, mapID, shardIdentity)
			if idErr != nil {
				return QAMap{}, idErr
			}
			shard := QAShard{SchemaVersion: 1, ID: shardID, AttemptID: attemptID, Kind: QAShardPrimary, Title: key, ChangedPaths: chunk, ContextPaths: context, BehavioralConcerns: concerns, ExpectationRefs: shardIdentity.ExpectationRefs, RiskTags: riskTags, ApprovedChecks: append([]QAApprovedCheckRef(nil), input.ApprovedChecks...), Phase: phase, Blocker: blocker}
			shards = append(shards, shard)
			for _, path := range chunk {
				owners[path] = shardID
			}
		}
	}
	if len(shards) > input.Settings.Budgets.PrimaryShards {
		return QAMap{}, NewQAError(QAErrorBudgetExhausted, "map", "primary shard limit exceeded", nil)
	}
	primaryCount := len(shards)
	if primaryCount > 1 && input.Settings.Budgets.BoundaryShards > 0 {
		overlap := make([]string, 0, primaryCount)
		for i := 0; i < primaryCount; i++ {
			overlap = append(overlap, shards[i].ChangedPaths[0])
		}
		if len(overlap) > input.Settings.Budgets.ContextPathsPerShard {
			overlap = overlap[:input.Settings.Budgets.ContextPathsPerShard]
		}
		boundaryIdentity := QAShardIdentity{Kind: QAShardBoundary, ContextPaths: overlap, BehavioralConcerns: []string{"cross-package integration"}, ExpectationRefs: normalizeQAStrings(input.ExpectationRefs)}
		boundaryID, idErr := NewQAShardID(input.Project, input.Sprint, mapID, boundaryIdentity)
		if idErr != nil {
			return QAMap{}, idErr
		}
		shards = append(shards, QAShard{SchemaVersion: 1, ID: boundaryID, AttemptID: attemptID, Kind: QAShardBoundary, Title: "cross-package integration", ContextPaths: overlap, OverlapPaths: overlap, BoundaryReason: "cross-package", BehavioralConcerns: boundaryIdentity.BehavioralConcerns, ExpectationRefs: boundaryIdentity.ExpectationRefs, RiskTags: qaTagsForPaths(input.RiskTags, overlap), ApprovedChecks: append([]QAApprovedCheckRef(nil), input.ApprovedChecks...), Phase: QAPhaseMapped})
	}
	if len(shards) > input.Settings.Budgets.TotalShards || len(shards) > input.Settings.Budgets.PendingEntries {
		return QAMap{}, NewQAError(QAErrorBudgetExhausted, "map", "planned shard or pending queue limit exceeded", nil)
	}
	sort.Slice(shards, func(i, j int) bool { return shards[i].ID < shards[j].ID })
	inputRefs := append([]QAArtifactRef(nil), input.InputRefs...)
	sort.Slice(inputRefs, func(i, j int) bool {
		if inputRefs[i].Path == inputRefs[j].Path {
			return inputRefs[i].Digest < inputRefs[j].Digest
		}
		return inputRefs[i].Path < inputRefs[j].Path
	})
	sources := append([]QAEffectiveSource(nil), input.Settings.Sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Field < sources[j].Field })
	coverage := QACoverage{ChangedPaths: paths, PrimaryOwners: owners, BoundaryOverlaps: map[string][]string{}}
	for _, shard := range shards {
		if shard.Kind == QAShardBoundary {
			coverage.BoundaryOverlaps[shard.ID] = append([]string(nil), shard.OverlapPaths...)
		}
		if shard.Blocker != nil {
			coverage.BlockedPaths = append(coverage.BlockedPaths, shard.ChangedPaths...)
		}
	}
	if len(coverage.BoundaryOverlaps) == 0 {
		coverage.BoundaryOverlaps = nil
	}
	coverage.BlockedPaths = normalizeQAStrings(coverage.BlockedPaths)
	for i := range shards {
		shards[i].ContextBlockIDs = qaContextBlockIDs(qaProjectFoundation(input.Foundation, shards[i]))
	}
	result := QAMap{SchemaVersion: 1, ID: mapID, Project: input.Project, Sprint: input.Sprint, SemanticAttemptID: attemptID, GovernedInputFingerprint: input.GovernedInputFingerprint, ImplementationFingerprint: input.ImplementationFingerprint, ReviewFingerprint: input.ReviewFingerprint, PolicyFingerprint: input.PolicyFingerprint, CheckCatalogFingerprint: input.CheckCatalogFingerprint, Budgets: input.Settings.Budgets, EffectiveSources: sources, Target: input.Target, Coverage: coverage, Shards: shards, InputRefs: inputRefs, Foundation: input.Foundation, Mapper: &QAMapperRecord{Executor: "deterministic", Fallback: true, Reason: "semantic mapper has not run"}}
	if err := ValidateQAMap(result); err != nil {
		return QAMap{}, NewQAError(QAErrorInvalidState, "map", err.Error(), err)
	}
	return result, nil
}

func NormalizedQAMapBytes(value QAMap) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func validateQAPath(path string) error {
	if filepath.IsAbs(path) || path == "." || path == ".." || strings.HasPrefix(filepath.ToSlash(filepath.Clean(filepath.FromSlash(path))), "../") || strings.ContainsAny(path, "\x00\r\n") {
		return fmt.Errorf("unsafe changed path %q", path)
	}
	return nil
}

func qaBehaviorGroup(path string) (string, bool) {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	ext := strings.ToLower(filepath.Ext(clean))
	switch ext {
	case ".go", ".md", ".html", ".js", ".css", ".yml", ".yaml", ".json", ".sh", ".sql":
	default:
		return "", false
	}
	parts := strings.Split(clean, "/")
	if len(parts) == 1 {
		return "root/" + ext, true
	}
	if parts[0] == "internal" || parts[0] == "cmd" {
		if len(parts) > 1 {
			return parts[0] + "/" + parts[1], true
		}
	}
	return parts[0], true
}

func qaContextForPaths(context map[string][]string, paths []string, limit int) []string {
	var values []string
	for _, path := range paths {
		values = append(values, context[path]...)
	}
	values = normalizeQAStrings(values)
	if len(values) > limit {
		values = values[:limit]
	}
	return values
}

func qaTagsForPaths(tags map[string][]string, paths []string) []string {
	var values []string
	for _, path := range paths {
		values = append(values, tags[path]...)
	}
	return normalizeQAStrings(values)
}

func qaAdjacentContext(target string, changed []string, limit int) map[string][]string {
	result := map[string][]string{}
	for _, path := range changed {
		var candidates []string
		if strings.HasSuffix(path, "_test.go") {
			candidates = append(candidates, strings.TrimSuffix(path, "_test.go")+".go")
		} else if strings.HasSuffix(path, ".go") {
			candidates = append(candidates, strings.TrimSuffix(path, ".go")+"_test.go")
		}
		for _, candidate := range candidates {
			if len(result[path]) >= limit {
				break
			}
			if info, err := os.Stat(filepath.Join(target, filepath.FromSlash(candidate))); err == nil && info.Mode().IsRegular() {
				result[path] = append(result[path], candidate)
			}
		}
	}
	return result
}

func qaRiskTags(paths []string) map[string][]string {
	result := map[string][]string{}
	for _, path := range paths {
		lower := strings.ToLower(filepath.ToSlash(path))
		var tags []string
		if strings.Contains(lower, "security") || strings.Contains(lower, "permission") || strings.Contains(lower, "config") {
			tags = append(tags, "security")
		}
		if strings.Contains(lower, "state") || strings.Contains(lower, "store") || strings.Contains(lower, "migration") {
			tags = append(tags, "persistence")
		}
		if strings.HasPrefix(lower, "internal/app/") || strings.HasPrefix(lower, "internal/web/") || strings.HasPrefix(lower, "cmd/") {
			tags = append(tags, "public-api")
		}
		if strings.Contains(lower, "runtime") || strings.Contains(lower, "process") {
			tags = append(tags, "runtime")
		}
		if strings.Contains(lower, "_test.") {
			tags = append(tags, "test")
		}
		result[path] = normalizeQAStrings(tags)
	}
	return result
}

func fingerprintQAValue(value any) (string, error) {
	data, err := canonicalQAJSON(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func qaGitIdentity(target string) (head, index, worktree string) {
	read := func(args ...string) string {
		output, err := exec.Command("git", append([]string{"-C", target}, args...)...).Output()
		if err != nil {
			return ""
		}
		return strings.TrimSpace(string(output))
	}
	head = read("rev-parse", "HEAD")
	index = hashBytes([]byte(read("diff", "--cached", "--no-ext-diff")))
	worktree = hashBytes([]byte(read("status", "--short", "--untracked-files=all")))
	return head, index, worktree
}

func qaWorkspaceInputRef(root string, path string) QAArtifactRef {
	full, err := workspace.ResolveInside(root, path)
	if err != nil {
		return QAArtifactRef{Path: path, Digest: "invalid"}
	}
	digest, err := hashFile(full)
	if err != nil {
		return QAArtifactRef{Path: path, Digest: "missing"}
	}
	return QAArtifactRef{Path: filepath.ToSlash(path), Digest: digest}
}
