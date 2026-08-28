package sprint

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	pprocess "github.com/Antonio7098/ultraplan-go/internal/platform/process"
	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type QACheckDescriptor struct {
	ID               string        `json:"id"`
	Executable       string        `json:"executable"`
	Args             []string      `json:"args"`
	WorkingDirectory string        `json:"working_directory"`
	Environment      []string      `json:"environment"`
	Timeout          time.Duration `json:"timeout"`
	OutputLimit      int           `json:"output_limit"`
	RequireEmptyOut  bool          `json:"require_empty_stdout,omitempty"`
	Fingerprint      string        `json:"fingerprint"`
}

type QAInvestigatorPacket struct {
	SchemaVersion             int                  `json:"schema_version"`
	MapID                     string               `json:"map_id"`
	ShardID                   string               `json:"shard_id"`
	ChangedPaths              []string             `json:"changed_paths"`
	ContextPaths              []string             `json:"context_paths"`
	BehavioralConcerns        []string             `json:"behavioral_concerns"`
	ExpectationRefs           []string             `json:"expectation_refs"`
	ApprovedChecks            []QAApprovedCheckRef `json:"approved_checks"`
	ImplementationFingerprint string               `json:"implementation_fingerprint"`
	Budgets                   QABudgets            `json:"budgets"`
}

type QAChallengerTheory struct {
	ID                  string          `json:"id"`
	Claim               string          `json:"claim"`
	VerificationSurface string          `json:"verification_surface"`
	ExpectationRefs     []string        `json:"expectation_refs"`
	Outcome             QATheoryOutcome `json:"outcome"`
	OutcomeReason       string          `json:"outcome_reason"`
}

type QAChallengerPacket struct {
	SchemaVersion             int                  `json:"schema_version"`
	MapID                     string               `json:"map_id"`
	ImplementationFingerprint string               `json:"implementation_fingerprint"`
	Theories                  []QAChallengerTheory `json:"theories"`
	ReturnedTheories          int                  `json:"returned_theories"`
	TotalTheories             int                  `json:"total_theories"`
	MaxChallenges             int                  `json:"max_challenges"`
}

func ApprovedQAChecks(target string, changedPaths []string, budgets QABudgets) ([]QACheckDescriptor, error) {
	if !filepath.IsAbs(target) {
		return nil, fmt.Errorf("QA check target must be absolute")
	}
	var goPaths []string
	for _, path := range normalizeQAStrings(changedPaths) {
		if err := validateQAPath(path); err != nil {
			return nil, err
		}
		if strings.HasSuffix(path, ".go") {
			goPaths = append(goPaths, path)
		}
	}
	if len(goPaths) == 0 {
		return nil, nil
	}
	descriptors := []QACheckDescriptor{
		{ID: "go-format-diff", Executable: "gofmt", Args: append([]string{"-d"}, goPaths...), WorkingDirectory: filepath.Clean(target), Timeout: budgets.CommandTimeout, OutputLimit: budgets.CommandOutputBytes, RequireEmptyOut: true},
		{ID: "go-source-integrity", Args: append([]string(nil), goPaths...), WorkingDirectory: filepath.Clean(target), Timeout: budgets.CommandTimeout, OutputLimit: budgets.CommandOutputBytes},
	}
	for i := range descriptors {
		if err := validateQACheckDescriptor(target, descriptors[i], budgets); err != nil {
			return nil, err
		}
		fingerprint := descriptors[i]
		fingerprint.Fingerprint = ""
		digest, err := fingerprintQAValue(fingerprint)
		if err != nil {
			return nil, err
		}
		descriptors[i].Fingerprint = digest
	}
	return descriptors, nil
}

func validateQACheckDescriptor(target string, descriptor QACheckDescriptor, budgets QABudgets) error {
	if strings.TrimSpace(descriptor.ID) == "" {
		return fmt.Errorf("QA check requires an ID")
	}
	if strings.TrimSpace(descriptor.Executable) == "" && descriptor.ID != "go-source-integrity" {
		return fmt.Errorf("QA check requires an executable")
	}
	executable := strings.ToLower(filepath.Base(descriptor.Executable))
	switch executable {
	case "sh", "bash", "zsh", "fish", "cmd", "powershell", "pwsh", "python", "python3", "perl", "ruby", "node", "git":
		return fmt.Errorf("QA check executable %q is prohibited", executable)
	}
	if filepath.Clean(descriptor.WorkingDirectory) != filepath.Clean(target) {
		return fmt.Errorf("QA check working directory must equal the target root")
	}
	if descriptor.Timeout <= 0 || descriptor.Timeout > budgets.CommandTimeout || descriptor.OutputLimit <= 0 || descriptor.OutputLimit > budgets.CommandOutputBytes {
		return fmt.Errorf("QA check exceeds command timeout or output budget")
	}
	for _, arg := range descriptor.Args {
		if strings.ContainsAny(arg, "\x00\r\n|;&><`$()") {
			return fmt.Errorf("QA check argument contains shell or redirection syntax")
		}
		if arg == "-w" || strings.HasPrefix(arg, "--write") {
			return fmt.Errorf("QA check requests a write mode")
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		full := filepath.Join(target, filepath.FromSlash(arg))
		if filepath.IsAbs(arg) || !inside(target, full) {
			return fmt.Errorf("QA check path escapes the target")
		}
	}
	for _, name := range descriptor.Environment {
		if !validQAEnvironmentName(name) {
			return fmt.Errorf("QA check environment name %q is not allowlisted", name)
		}
	}
	return nil
}

func validQAEnvironmentName(name string) bool {
	switch name {
	case "LANG", "LC_ALL", "TZ":
		return true
	default:
		return false
	}
}

func (s Service) RenderQAInvestigatorPrompt(qaMap QAMap, shard QAShard) (string, error) {
	if shard.AttemptID != qaMap.SemanticAttemptID {
		return "", fmt.Errorf("QA shard does not belong to the selected map")
	}
	packet := QAInvestigatorPacket{SchemaVersion: 1, MapID: qaMap.ID, ShardID: shard.ID, ChangedPaths: append([]string(nil), shard.ChangedPaths...), ContextPaths: append([]string(nil), shard.ContextPaths...), BehavioralConcerns: append([]string(nil), shard.BehavioralConcerns...), ExpectationRefs: append([]string(nil), shard.ExpectationRefs...), ApprovedChecks: append([]QAApprovedCheckRef(nil), shard.ApprovedChecks...), ImplementationFingerprint: qaMap.ImplementationFingerprint, Budgets: qaMap.Budgets}
	data, err := canonicalQAJSON(packet)
	if err != nil {
		return "", err
	}
	prompt := `# Read-only QA investigator

Inspect only the assigned changed and context paths. You may read, list, and search those paths. You cannot write files, create tests or fixtures, invoke a shell, mutate Git, promote issues, or repair code. If more context is essential, return a bounded context request. If an existing product-owned check is useful, request its ID. Do not invent an executable, arguments, environment, path, prompt, or output location.

Return exactly one JSON object with no Markdown fence or surrounding text. Unknown fields are rejected. All five top-level fields must appear. schema_version must be 1. theories, evidence, context_requests, and check_requests must be JSON arrays. Return at least one falsifiable theory. If assigned context is insufficient, retain an inconclusive theory describing the missing evidence and include a bounded context request; an empty theories array is rejected. Each theory draft must contain claim, basis, verification_surface, expectation_refs, severity_if_confirmed, confirmation_condition, refutation_condition, inconclusive_condition, safe_evidence_strategy, outcome, and outcome_reason. The product assigns IDs, implementation fingerprints, attempt history, and timestamps after validation. Outcomes are confirmed, refuted, invalid, inconclusive, blocked, cross_shard, or not_applicable. Evidence entries contain kind, summary, paths, check_id, and output_digest. Context requests contain id, paths, reason, approved, and optional denied_reason; approved must be false. Check requests contain only id and fingerprint copied exactly from approved_checks.

Assigned packet:
` + string(data) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		return "", NewQAError(QAErrorBudgetExhausted, "prepare prompt", "investigator prompt exceeds the map budget", nil)
	}
	return prompt, nil
}

func (s Service) QAInvestigatorRequest(qaMap QAMap, shard QAShard, target string) (pruntime.Request, error) {
	settings, err := s.effectiveQASettings()
	if err != nil {
		return pruntime.Request{}, err
	}
	prompt, err := s.RenderQAInvestigatorPrompt(qaMap, shard)
	if err != nil {
		return pruntime.Request{}, err
	}
	req := s.runtimeRequest(prompt, map[string]string{"project": qaMap.Project, "sprint": qaMap.Sprint, "stage": string(VerificationPhaseQA), "shard": shard.ID, "map": qaMap.ID})
	runtimeSettings := settings.RuntimeFor("investigator")
	provider, model := splitProviderModel(runtimeSettings.Model)
	req.Provider, req.Model = provider, model
	req.Metadata["variant"] = runtimeSettings.Variant
	req.Metadata["reasoning_effort"] = runtimeSettings.Variant
	req.WorkDir = filepath.Clean(target)
	req.Timeout = settings.Budgets.ShardTimeout
	req.Sandbox = "read_only"
	req.Permissions = "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "list": "allow", "search": "allow", "glob": "allow", "write": "deny", "edit": "deny", "patch": "deny", "bash": "deny", "shell": "deny"}}
	paths := append(append([]string(nil), shard.ChangedPaths...), shard.ContextPaths...)
	for _, rel := range normalizeQAStrings(paths) {
		if err := validateQAPath(rel); err != nil {
			return pruntime.Request{}, err
		}
		full := filepath.Join(target, filepath.FromSlash(rel))
		if !inside(target, full) {
			return pruntime.Request{}, fmt.Errorf("QA investigator path escapes target")
		}
		req.Policy.PathRules = append(req.Policy.PathRules, pruntime.PermissionPathRule{Path: full, Action: "allow"})
	}
	sort.Slice(req.Policy.PathRules, func(i, j int) bool { return req.Policy.PathRules[i].Path < req.Policy.PathRules[j].Path })
	return req, nil
}

// QAChallengerRequest creates an optional, bounded, read-only semantic
// challenge pass over retained theory summaries. Its output is still only a
// proposal; BuildQAChallenge and SynthesizeQAWithChallenges own validation.
func (s Service) QAChallengerRequest(qaMap QAMap, shards []QAShard, target string) (pruntime.Request, error) {
	settings, err := s.effectiveQASettings()
	if err != nil {
		return pruntime.Request{}, err
	}
	theories := make([]QAChallengerTheory, 0)
	total := 0
	for _, shard := range shards {
		for _, theory := range shard.Theories {
			total++
			theories = append(theories, QAChallengerTheory{ID: theory.ID, Claim: theory.Claim, VerificationSurface: theory.VerificationSurface, ExpectationRefs: normalizeQAStrings(theory.ExpectationRefs), Outcome: theory.Outcome, OutcomeReason: theory.OutcomeReason})
		}
	}
	sort.Slice(theories, func(i, j int) bool { return theories[i].ID < theories[j].ID })
	limit := qaMap.Budgets.FollowUpShards * qaMap.Budgets.TheoriesPerShard
	if len(theories) > limit {
		theories = theories[:limit]
	}
	packet := QAChallengerPacket{SchemaVersion: QASchemaVersion, MapID: qaMap.ID, ImplementationFingerprint: qaMap.ImplementationFingerprint, Theories: theories, ReturnedTheories: len(theories), TotalTheories: total, MaxChallenges: qaMap.Budgets.FollowUpShards}
	data, err := canonicalQAJSON(packet)
	if err != nil {
		return pruntime.Request{}, err
	}
	prompt := `# Read-only QA challenger

Challenge only the frozen theory summaries below. Do not change an outcome, create an issue, propose a repair, invoke a command, or request additional context. Return at most max_challenges JSON records with theory_ids, claim, basis, safe_evidence_strategy, and evidence_refs. The product assigns schema version, map identity, and deterministic IDs after validation.

Frozen packet:
` + string(data) + "\n"
	if len(prompt) > qaMap.Budgets.PromptBytes {
		return pruntime.Request{}, NewQAError(QAErrorBudgetExhausted, "prepare challenger", "challenger prompt exceeds the map budget", nil)
	}
	req := s.runtimeRequest(prompt, map[string]string{"project": qaMap.Project, "sprint": qaMap.Sprint, "stage": string(VerificationPhaseQA), "map": qaMap.ID, "role": "challenger"})
	runtimeSettings := settings.RuntimeFor("challenger")
	provider, model := splitProviderModel(runtimeSettings.Model)
	req.Provider, req.Model = provider, model
	req.Metadata["variant"] = runtimeSettings.Variant
	req.Metadata["reasoning_effort"] = runtimeSettings.Variant
	req.WorkDir = filepath.Clean(target)
	req.Timeout = settings.Budgets.ShardTimeout
	req.Sandbox = "read_only"
	req.Permissions = "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	req.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "deny", "list": "deny", "search": "deny", "glob": "deny", "write": "deny", "edit": "deny", "patch": "deny", "bash": "deny", "shell": "deny"}}
	return req, nil
}

func (s Service) RunApprovedQACheck(ctx context.Context, qaMap QAMap, descriptor QACheckDescriptor, requested QAApprovedCheckRef) (QACommandSummary, error) {
	if descriptor.ID != requested.ID || descriptor.Fingerprint != requested.Fingerprint {
		return QACommandSummary{}, NewQAError(QAErrorPermissionDenied, "run check", "requested check is not owned by the current map", nil)
	}
	owned := false
	for _, shard := range qaMap.Shards {
		for _, check := range shard.ApprovedChecks {
			if check == requested {
				owned = true
				break
			}
		}
	}
	if !owned {
		return QACommandSummary{}, NewQAError(QAErrorPermissionDenied, "run check", "requested check is absent from the current map", nil)
	}
	if err := validateQACheckDescriptor(descriptor.WorkingDirectory, descriptor, qaMap.Budgets); err != nil {
		return QACommandSummary{}, NewQAError(QAErrorPermissionDenied, "run check", err.Error(), err)
	}
	before, err := targetIdentity(descriptor.WorkingDirectory)
	if err != nil {
		return QACommandSummary{}, NewQAError(QAErrorStaleInput, "run check", "cannot capture target identity", err)
	}
	if descriptor.ID == "go-source-integrity" {
		if err := validateGoSourcePaths(descriptor.WorkingDirectory, descriptor.Args); err != nil {
			return QACommandSummary{CheckID: descriptor.ID, DescriptorFingerprint: descriptor.Fingerprint, ExitCode: 1}, NewQAError(QAErrorInvalidState, "run check", "Go source integrity check failed", err)
		}
		after, identityErr := targetIdentity(descriptor.WorkingDirectory)
		if identityErr != nil || after != before {
			return QACommandSummary{}, NewQAError(QAErrorPermissionDenied, "run check", "target identity changed during the approved check", identityErr)
		}
		return QACommandSummary{CheckID: descriptor.ID, DescriptorFingerprint: descriptor.Fingerprint, ExitCode: 0}, nil
	}
	env := make([]string, 0, len(descriptor.Environment))
	for _, name := range descriptor.Environment {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	result, runErr := s.processRunner.Run(ctx, pprocess.Request{Executable: descriptor.Executable, Args: append([]string(nil), descriptor.Args...), Dir: descriptor.WorkingDirectory, Env: env, Timeout: descriptor.Timeout, StdoutLimit: descriptor.OutputLimit, StderrLimit: descriptor.OutputLimit, CleanupGrace: qaMap.Budgets.CleanupTimeout})
	after, identityErr := targetIdentity(descriptor.WorkingDirectory)
	if identityErr != nil || after != before {
		return QACommandSummary{}, NewQAError(QAErrorPermissionDenied, "run check", "target identity changed during the approved check", identityErr)
	}
	summary := QACommandSummary{CheckID: descriptor.ID, DescriptorFingerprint: descriptor.Fingerprint, ExitCode: result.ExitCode, Duration: result.Duration, StdoutDigest: hashBytes([]byte(result.Stdout)), StderrDigest: hashBytes([]byte(result.Stderr)), OutputBytes: len(result.Stdout) + len(result.Stderr), Truncated: result.StdoutTruncated || result.StderrTruncated}
	if runErr != nil {
		return summary, NewQAError(QAErrorRuntimeUnavailable, "run check", "approved check did not complete successfully", runErr)
	}
	if summary.OutputBytes > descriptor.OutputLimit*2 || summary.Truncated {
		return summary, NewQAError(QAErrorBudgetExhausted, "run check", "approved check output limit was exhausted", nil)
	}
	if descriptor.RequireEmptyOut && len(result.Stdout) > 0 {
		return summary, NewQAError(QAErrorInvalidState, "run check", "approved check produced disallowed standard output", nil)
	}
	return summary, nil
}

func validateGoSourcePaths(root string, paths []string) error {
	files := token.NewFileSet()
	for _, rel := range paths {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if !inside(root, path) {
			return fmt.Errorf("Go source path escapes the target")
		}
		if _, err := parser.ParseFile(files, path, nil, parser.AllErrors); err != nil {
			return err
		}
	}
	return nil
}
