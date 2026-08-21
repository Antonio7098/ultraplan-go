package sprint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/project"
)

type smokeManifest struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	ProtocolVersion string               `json:"protocolVersion"`
	Harness         smokeHarnessIdentity `json:"harness"`
	Executable      string               `json:"executable"`
	Args            []string             `json:"args"`
	CWD             string               `json:"cwd"`
	Commands        smokeCommands        `json:"commands"`
	Evidence        smokeEvidenceRoots   `json:"evidence"`
	Authoring       smokeAuthoring       `json:"authoring"`
	Capabilities    []string             `json:"capabilities"`
	Environment     []string             `json:"environment"`
	Defaults        smokeDefaults        `json:"defaults"`
}

type smokeHarnessIdentity struct{ ID, Version string }
type smokeCommands struct{ Discover, Run []string }
type smokeEvidenceRoots struct{ Runs, Issues string }
type smokeAuthoring struct {
	Paths []string `json:"paths"`
}
type smokeDefaults struct{ Level, Timeout, DurationClass, CostClass string }

type smokeDiscovery struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	ProtocolVersion string               `json:"protocolVersion"`
	HarnessID       string               `json:"harnessId"`
	Levels          []smokeLevel         `json:"levels"`
	Suites          []smokeSuite         `json:"suites"`
	Tests           []smokeTest          `json:"tests"`
	SprintMappings  []smokeSprintMapping `json:"sprintMappings"`
	Prerequisites   []smokePrerequisite  `json:"prerequisites"`
	EvidenceSchema  int                  `json:"evidenceSchema"`
}
type smokeLevel struct {
	ID            string   `json:"id"`
	Suites        []string `json:"suites"`
	DurationClass string   `json:"durationClass"`
	CostClass     string   `json:"costClass"`
}
type smokeSuite struct {
	ID                            string `json:"id"`
	Tests, Sprints, Prerequisites []string
	DurationClass                 string `json:"durationClass"`
	CostClass                     string `json:"costClass"`
}
type smokeTest struct {
	ID, Suite          string
	EquivalentComplete bool     `json:"equivalentComplete"`
	Coverage           []string `json:"coverage"`
}
type smokeSprintMapping struct {
	Sprint                  string
	Suites                  []string
	Complete, NotApplicable bool
	Rationale               string
	RequiredCoverage        []string `json:"requiredCoverage"`
}
type smokePrerequisite struct{ ID, Description, Status, Action string }

type smokeRunResponse struct {
	SchemaVersion   int                 `json:"schemaVersion"`
	ProtocolVersion string              `json:"protocolVersion"`
	HarnessID       string              `json:"harnessId"`
	RunID           string              `json:"runId"`
	ScopeKind       string              `json:"scopeKind"`
	Scope           []string            `json:"scope"`
	Counts          SmokeCountsWire     `json:"counts"`
	DurationMs      int64               `json:"durationMs"`
	Runtime         string              `json:"runtime,omitempty"`
	Model           string              `json:"model,omitempty"`
	Evidence        []smokeEvidenceWire `json:"evidence"`
	Issues          []SmokeIssue        `json:"issues,omitempty"`
	Tests           []SmokeTestResult   `json:"tests"`
}
type SmokeCountsWire struct{ Total, Passed, Failed, Skipped, Errors int }
type smokeEvidenceWire struct{ Kind, Path, SHA256 string }

type smokePrepared struct {
	Sprint                                                                   Sprint
	Manifest                                                                 smokeManifest
	HarnessRoot, ManifestPath, Executable, CWD, RunsRoot, IssuesRoot, Target string
	Review                                                                   ReviewStageState
}

type smokeSelection struct {
	Kind                     string
	IDs                      []string
	Rationale                string
	Prerequisites            []string
	Verdict                  SmokeVerdict
	NextAction               string
	DiagnosticOnly           bool
	DurationClass, CostClass string
}

func (s Service) prepareSmokeStatic(projectRef, sprintRef string, req SmokeRequest) (smokePrepared, error) {
	sp, inputs, catalog, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return smokePrepared{}, err
	}
	entries := make([]project.CatalogEntry, 0, 1)
	for _, entry := range catalog.Entries {
		if entry.Section == project.SectionSmokeHarnesses {
			entries = append(entries, entry)
		}
	}
	if len(entries) != 1 {
		return smokePrepared{}, smokeError("smoke_catalog", "catalog", "exactly one smoke harness is required", "Add one current Smoke Harnesses row to project-index.md.", nil)
	}
	entry := entries[0]
	target, targetFindings := ResolveExecuteTarget(inputs.ProjectIndex)
	if len(targetFindings) > 0 || target.Path == "" {
		return smokePrepared{}, smokeError("smoke_target", "catalog", "target implementation directory is missing or invalid", "Set one absolute Target Implementation Directory in project-index.md.", nil)
	}
	root, err := canonicalDirectory(entry.Path)
	if err != nil {
		return smokePrepared{}, smokeError("smoke_harness_root", "containment", "harness root is unavailable", "Restore the cataloged harness directory.", err)
	}
	manifestPath := entry.Manifest
	if !filepath.IsAbs(manifestPath) {
		manifestPath = filepath.Join(root, filepath.FromSlash(manifestPath))
	}
	manifestPath, err = canonicalFileInside(root, manifestPath)
	if err != nil {
		return smokePrepared{}, smokeError("smoke_manifest_path", "containment", "manifest is unavailable or escapes the harness", "Use a manifest contained by the harness root.", err)
	}
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return smokePrepared{}, smokeError("smoke_manifest_read", "protocol", "manifest cannot be read", "Restore the manifest and its permissions.", err)
	}
	var manifest smokeManifest
	if err := decodeOneJSON(string(data), &manifest); err != nil {
		return smokePrepared{}, smokeError("smoke_manifest_malformed", "protocol", "manifest is malformed JSON", "Write a valid protocol-v1 manifest.", err)
	}
	if err := validateSmokeManifest(manifest); err != nil {
		return smokePrepared{}, err
	}
	executable := manifest.Executable
	if !filepath.IsAbs(executable) {
		executable = filepath.Join(root, filepath.FromSlash(executable))
	}
	executable, err = canonicalFileInside(root, executable)
	if err != nil {
		return smokePrepared{}, smokeError("smoke_executable_path", "containment", "executable is unavailable or escapes the harness", "Use a contained executable.", err)
	}
	cwd := manifest.CWD
	if !filepath.IsAbs(cwd) {
		cwd = filepath.Join(root, filepath.FromSlash(cwd))
	}
	cwd, err = canonicalDirectoryInside(root, cwd)
	if err != nil {
		return smokePrepared{}, smokeError("smoke_cwd_path", "containment", "working directory is unavailable or escapes the harness", "Use a contained working directory.", err)
	}
	runs, err := canonicalDirectoryInside(root, filepath.Join(root, filepath.FromSlash(manifest.Evidence.Runs)))
	if err != nil {
		return smokePrepared{}, smokeError("smoke_runs_path", "containment", "runs evidence root is invalid", "Create the contained runs directory.", err)
	}
	issues, err := canonicalDirectoryInside(root, filepath.Join(root, filepath.FromSlash(manifest.Evidence.Issues)))
	if err != nil {
		return smokePrepared{}, smokeError("smoke_issues_path", "containment", "issues evidence root is invalid", "Create the contained issues directory.", err)
	}
	state, err := LoadFlowState(s.root, sp)
	if err != nil {
		return smokePrepared{}, smokeError("smoke_review_state", "review_gate", "review flow state is unavailable", "Restore valid flow state and rerun review.", err)
	}
	if state.Review == nil {
		return smokePrepared{}, smokeError("smoke_review_missing", "review_gate", "a current review is required", "Run sprint review before smoke.", nil)
	}
	review := *state.Review
	reviewManifest, reviewFindings, reviewErr := s.PrepareReview(projectRef, sprintRef, ReviewRequest{})
	review.Stale = reviewErr != nil || len(reviewFindings) > 0 || (strictCompletedReviewSnapshotFreshness && reviewManifest.Fingerprint != review.Fingerprint)
	if !review.Stale {
		content, readErr := s.store.ReadArtifact(sp, StageReview)
		validationManifest := reviewManifest
		if !strictCompletedReviewSnapshotFreshness {
			validationManifest.Fingerprint = review.Fingerprint
		}
		review.Stale = readErr != nil || len(ValidateReviewContent(content, validationManifest)) > 0 || review.ArtifactDigest == "" || hashBytes([]byte(content)) != review.ArtifactDigest
	}
	if review.Stale {
		return smokePrepared{}, smokeError("smoke_review_stale", "review_gate", "review is stale", "Run sprint review again; stale reviews cannot be overridden.", nil)
	}
	switch review.Verdict {
	case ReviewPass, ReviewPassWithFindings:
	case ReviewFail, ReviewVerdictBlocked:
		if !req.ForceReview {
			return smokePrepared{}, smokeError("smoke_review_blocked", "review_gate", "review verdict blocks smoke", "Use --force-review only for an explicitly confirmed diagnostic run.", nil)
		}
		if !req.OverrideConfirmed || strings.TrimSpace(req.OverrideRationale) == "" {
			return smokePrepared{}, smokeError("smoke_review_override_confirmation", "review_gate", "diagnostic override requires explicit confirmation and rationale", "Pass the confirmation flag with a non-empty actor-neutral rationale.", nil)
		}
	default:
		return smokePrepared{}, smokeError("smoke_review_invalid", "review_gate", "review verdict is missing or unsupported", "Regenerate review.md.", nil)
	}
	return smokePrepared{Sprint: sp, Manifest: manifest, HarnessRoot: root, ManifestPath: manifestPath, Executable: executable, CWD: cwd, RunsRoot: runs, IssuesRoot: issues, Target: target.Path, Review: review}, nil
}

func validateSmokeManifest(m smokeManifest) error {
	if m.SchemaVersion != 1 || protocolMajor(m.ProtocolVersion) != SmokeProtocolMajor {
		return smokeError("smoke_protocol_unsupported", "protocol", fmt.Sprintf("unsupported manifest schema/protocol %d/%q", m.SchemaVersion, m.ProtocolVersion), "Install a protocol-v1 harness.", nil)
	}
	if m.Harness.ID == "" || m.Harness.Version == "" || m.Executable == "" || m.CWD == "" || len(m.Commands.Discover) == 0 || len(m.Commands.Run) == 0 || m.Evidence.Runs == "" || m.Evidence.Issues == "" {
		return smokeError("smoke_manifest_required", "protocol", "manifest is missing required fields", "Set harness identity, executable, cwd, commands, and evidence roots.", nil)
	}
	if m.Defaults.Timeout != "" {
		timeout, err := time.ParseDuration(m.Defaults.Timeout)
		if err != nil || timeout <= 0 || timeout > 24*time.Hour {
			return smokeError("smoke_manifest_timeout", "configuration", "manifest default timeout is invalid", "Use a positive Go duration no greater than 24h.", err)
		}
	}
	required := []string{"discovery", "run", "evidence-v1", "scope-mapping", "authoring-v1"}
	for _, capability := range required {
		if !contains(m.Capabilities, capability) {
			return smokeError("smoke_capability_missing", "protocol", "manifest lacks capability "+capability, "Upgrade the smoke harness.", nil)
		}
	}
	if len(m.Authoring.Paths) == 0 {
		return smokeError("smoke_authoring_paths", "protocol", "manifest has no smoke-authoring paths", "Declare the contained harness paths the smoke author may modify.", nil)
	}
	seenAuthoring := map[string]bool{}
	runsPath := filepath.ToSlash(filepath.Clean(m.Evidence.Runs))
	issuesPath := filepath.ToSlash(filepath.Clean(m.Evidence.Issues))
	for _, path := range m.Authoring.Paths {
		path = filepath.ToSlash(filepath.Clean(path))
		if path == "." || !safeRelPath(path) || seenAuthoring[path] || smokePathsOverlap(path, runsPath) || smokePathsOverlap(path, issuesPath) {
			return smokeError("smoke_authoring_paths", "protocol", "manifest contains an unsafe or duplicate smoke-authoring path", "Use unique contained paths outside the evidence roots.", nil)
		}
		for prior := range seenAuthoring {
			if smokePathsOverlap(path, prior) {
				return smokeError("smoke_authoring_paths", "protocol", "manifest contains overlapping smoke-authoring paths", "Use non-overlapping contained paths outside the evidence roots.", nil)
			}
		}
		seenAuthoring[path] = true
	}
	seen := map[string]bool{}
	for _, name := range m.Environment {
		if !validProtocolEnvName(name) || seen[name] {
			return smokeError("smoke_environment_invalid", "protocol", "manifest contains invalid or duplicate environment names", "Use unique uppercase environment names.", nil)
		}
		seen[name] = true
	}
	return nil
}

func validateSmokeDiscovery(d smokeDiscovery, m smokeManifest) error {
	if d.SchemaVersion != 1 || d.ProtocolVersion != m.ProtocolVersion || d.HarnessID != m.Harness.ID || d.EvidenceSchema != 1 {
		return smokeError("smoke_discovery_identity", "protocol", "discovery identity does not match the manifest", "Fix the harness protocol response.", nil)
	}
	prerequisites := map[string]bool{}
	levels := map[string]bool{}
	suites := map[string]bool{}
	tests := map[string]bool{}
	for _, prerequisite := range d.Prerequisites {
		if prerequisite.ID == "" || prerequisites[prerequisite.ID] {
			return smokeError("smoke_discovery_duplicate", "protocol", "discovery contains duplicate/empty prerequisite identity", "Fix discovery identities.", nil)
		}
		if prerequisite.Status != "available" && prerequisite.Status != "blocked" {
			return smokeError("smoke_prerequisite_status", "protocol", "discovery contains an unsupported prerequisite status", "Use available or blocked prerequisite status.", nil)
		}
		prerequisites[prerequisite.ID] = true
	}
	for _, level := range d.Levels {
		if level.ID == "" || levels[level.ID] {
			return smokeError("smoke_discovery_duplicate", "protocol", "discovery contains duplicate/empty level identity", "Fix discovery identities.", nil)
		}
		levels[level.ID] = true
	}
	for _, suite := range d.Suites {
		if suite.ID == "" || suites[suite.ID] {
			return smokeError("smoke_discovery_duplicate", "protocol", "discovery contains duplicate/empty suite identity", "Fix discovery identities.", nil)
		}
		suites[suite.ID] = true
	}
	for _, test := range d.Tests {
		if test.ID == "" || tests[test.ID] {
			return smokeError("smoke_discovery_duplicate", "protocol", "discovery contains duplicate/empty test identity", "Fix discovery identities.", nil)
		}
		tests[test.ID] = true
	}
	for _, level := range d.Levels {
		if len(level.Suites) == 0 {
			return smokeError("smoke_discovery_relationship", "protocol", "level has no suites", "Declare at least one suite for every level.", nil)
		}
		for _, id := range level.Suites {
			if !suites[id] {
				return smokeError("smoke_discovery_relationship", "protocol", "level references unknown suite "+id, "Fix discovery relationships.", nil)
			}
		}
	}
	testByID := map[string]smokeTest{}
	for _, test := range d.Tests {
		testByID[test.ID] = test
	}
	for _, suite := range d.Suites {
		for _, id := range suite.Tests {
			if !tests[id] {
				return smokeError("smoke_discovery_relationship", "protocol", "suite references unknown test "+id, "Fix discovery relationships.", nil)
			}
			if testByID[id].Suite != suite.ID {
				return smokeError("smoke_discovery_relationship", "protocol", "suite/test ownership is inconsistent", "Make each enumerated test name its containing suite.", nil)
			}
		}
		for _, id := range suite.Prerequisites {
			if !prerequisites[id] {
				return smokeError("smoke_discovery_relationship", "protocol", "suite references unknown prerequisite "+id, "Fix discovery relationships.", nil)
			}
		}
	}
	for _, test := range d.Tests {
		if !suites[test.Suite] {
			return smokeError("smoke_discovery_relationship", "protocol", "test references unknown suite "+test.Suite, "Fix discovery relationships.", nil)
		}
	}
	for _, mapping := range d.SprintMappings {
		if mapping.Sprint == "" || (mapping.Complete && mapping.NotApplicable) {
			return smokeError("smoke_discovery_relationship", "protocol", "sprint mapping is empty or contradictory", "Fix discovery mappings.", nil)
		}
		for _, id := range mapping.Suites {
			if !suites[id] {
				return smokeError("smoke_discovery_relationship", "protocol", "mapping references unknown suite "+id, "Fix discovery mappings.", nil)
			}
		}
		// Legacy mappings may be present for unrelated sprints. They cannot be
		// selected as complete until the smoke author upgrades them, but they do
		// not make discovery unusable for the sprint being authored now.
		if mapping.Complete && len(mapping.RequiredCoverage) > 0 {
			covered := map[string]bool{}
			for _, suiteID := range mapping.Suites {
				var selected smokeSuite
				for _, suite := range d.Suites {
					if suite.ID == suiteID {
						selected = suite
						break
					}
				}
				if len(selected.Tests) == 0 {
					return smokeError("smoke_discovery_coverage", "protocol", "complete sprint mapping references an empty suite", "Enumerate non-empty test identities for every complete sprint suite.", nil)
				}
				for _, testID := range selected.Tests {
					for _, coverageID := range testByID[testID].Coverage {
						covered[coverageID] = true
					}
				}
			}
			seenCoverage := map[string]bool{}
			for _, coverageID := range mapping.RequiredCoverage {
				if strings.TrimSpace(coverageID) == "" || seenCoverage[coverageID] {
					return smokeError("smoke_discovery_coverage", "protocol", "complete sprint mapping has empty or duplicate coverage IDs", "Use unique stable coverage IDs.", nil)
				}
				seenCoverage[coverageID] = true
				if !covered[coverageID] {
					return smokeError("smoke_discovery_coverage", "protocol", "complete sprint mapping leaves coverage unassigned: "+coverageID, "Add a deep-smoke test that declares this coverage ID.", nil)
				}
			}
		}
	}
	return nil
}

func smokeCoverageMapping(d smokeDiscovery, sprint string) *SmokeCoverageMapping {
	for _, mapping := range d.SprintMappings {
		if mapping.Sprint != sprint {
			continue
		}
		suites := map[string]bool{}
		for _, suite := range mapping.Suites {
			suites[suite] = true
		}
		out := &SmokeCoverageMapping{
			Sprint:           mapping.Sprint,
			Suites:           append([]string(nil), mapping.Suites...),
			Complete:         mapping.Complete,
			NotApplicable:    mapping.NotApplicable,
			Rationale:        mapping.Rationale,
			RequiredCoverage: append([]string(nil), mapping.RequiredCoverage...),
			Tests:            []SmokeCoverageTest{},
		}
		for _, test := range d.Tests {
			if suites[test.Suite] {
				out.Tests = append(out.Tests, SmokeCoverageTest{ID: test.ID, Suite: test.Suite, Coverage: append([]string(nil), test.Coverage...)})
			}
		}
		sort.Strings(out.Suites)
		sort.Slice(out.Tests, func(i, j int) bool { return out.Tests[i].ID < out.Tests[j].ID })
		return out
	}
	return nil
}

func selectSmoke(d smokeDiscovery, sprint string, req SmokeRequest) (smokeSelection, error) {
	levels, suites, tests := map[string]smokeLevel{}, map[string]smokeSuite{}, map[string]smokeTest{}
	for _, v := range d.Levels {
		levels[v.ID] = v
	}
	for _, v := range d.Suites {
		suites[v.ID] = v
	}
	for _, v := range d.Tests {
		tests[v.ID] = v
	}
	var mapping *smokeSprintMapping
	for i := range d.SprintMappings {
		if d.SprintMappings[i].Sprint == sprint {
			if mapping != nil {
				return smokeSelection{}, smokeError("smoke_mapping_duplicate", "scope", "multiple sprint mappings are ambiguous", "Keep one deterministic sprint mapping.", nil)
			}
			mapping = &d.SprintMappings[i]
		}
	}
	if mapping != nil && mapping.NotApplicable {
		return smokeSelection{Verdict: SmokeNotApplicable, Rationale: mapping.Rationale, NextAction: "No smoke action is required for this sprint."}, nil
	}
	checkPrereqs := func(ids []string) ([]string, bool) {
		var missing []string
		for _, id := range ids {
			found := false
			for _, p := range d.Prerequisites {
				if p.ID == id {
					found = true
					if p.Status != "available" {
						missing = append(missing, firstNonEmptyString(p.Description, p.ID)+firstSuffix(p.Action, "; action: "))
					}
				}
			}
			if !found {
				missing = append(missing, id+"; action: fix the discovery prerequisite relationship")
			}
		}
		return missing, len(missing) == 0
	}
	if req.Level != "" {
		level, ok := levels[req.Level]
		if !ok {
			return smokeSelection{}, smokeError("smoke_scope_unknown", "scope", "unknown level "+req.Level, "Choose a discovered level.", nil)
		}
		var prereqs []string
		for _, id := range level.Suites {
			prereqs = append(prereqs, suites[id].Prerequisites...)
		}
		missing, ok := checkPrereqs(prereqs)
		if !ok {
			return smokeSelection{Verdict: SmokeBlockedVerdict, Prerequisites: missing, NextAction: "Satisfy the listed prerequisites and rerun smoke."}, nil
		}
		complete := mapping != nil && mapping.Complete && len(mapping.RequiredCoverage) > 0 && containsAll(level.Suites, mapping.Suites)
		return smokeSelection{Kind: "level", IDs: []string{level.ID}, Rationale: "explicit level override", DiagnosticOnly: !complete, DurationClass: level.DurationClass, CostClass: level.CostClass}, nil
	}
	if req.Suite != "" {
		suite, ok := suites[req.Suite]
		if !ok {
			return smokeSelection{}, smokeError("smoke_scope_unknown", "scope", "unknown suite "+req.Suite, "Choose a discovered suite.", nil)
		}
		missing, ok := checkPrereqs(suite.Prerequisites)
		if !ok {
			return smokeSelection{Verdict: SmokeBlockedVerdict, Prerequisites: missing, NextAction: "Satisfy the listed prerequisites and rerun smoke."}, nil
		}
		complete := mapping != nil && mapping.Complete && len(mapping.RequiredCoverage) > 0 && len(mapping.Suites) == 1 && mapping.Suites[0] == suite.ID
		return smokeSelection{Kind: "suite", IDs: []string{suite.ID}, Rationale: "explicit suite override", DiagnosticOnly: !complete, DurationClass: suite.DurationClass, CostClass: suite.CostClass}, nil
	}
	if req.Test != "" {
		test, ok := tests[req.Test]
		if !ok {
			return smokeSelection{}, smokeError("smoke_scope_unknown", "scope", "unknown test "+req.Test, "Choose a discovered test.", nil)
		}
		suite := suites[test.Suite]
		missing, ok := checkPrereqs(suite.Prerequisites)
		if !ok {
			return smokeSelection{Verdict: SmokeBlockedVerdict, Prerequisites: missing, NextAction: "Satisfy the listed prerequisites and rerun smoke."}, nil
		}
		return smokeSelection{Kind: "test", IDs: []string{test.ID}, Rationale: "explicit diagnostic test override", DiagnosticOnly: true, DurationClass: suite.DurationClass, CostClass: suite.CostClass}, nil
	}
	if mapping == nil || !mapping.Complete || len(mapping.Suites) == 0 || len(mapping.RequiredCoverage) == 0 {
		return smokeSelection{Verdict: SmokeBlockedVerdict, Rationale: "discovery cannot prove enumerated deep-smoke coverage for this sprint", NextAction: "Update the harness with required coverage IDs and non-empty sprint-specific tests, then rerun smoke."}, nil
	}
	ids := append([]string(nil), mapping.Suites...)
	sort.Strings(ids)
	var prereqs []string
	for _, id := range ids {
		suite, ok := suites[id]
		if !ok {
			return smokeSelection{}, smokeError("smoke_mapping_unknown", "scope", "mapping references unknown suite "+id, "Fix discovery mappings.", nil)
		}
		prereqs = append(prereqs, suite.Prerequisites...)
	}
	missing, ok := checkPrereqs(prereqs)
	if !ok {
		return smokeSelection{Verdict: SmokeBlockedVerdict, Prerequisites: missing, Rationale: mapping.Rationale, NextAction: "Satisfy the listed prerequisites and rerun smoke."}, nil
	}
	return smokeSelection{Kind: "suite", IDs: ids, Rationale: firstNonEmptyString(mapping.Rationale, "narrowest complete sprint mapping")}, nil
}

func protocolMajor(value string) int {
	first := strings.SplitN(value, ".", 2)[0]
	n, _ := strconv.Atoi(first)
	return n
}
func contains(values []string, wanted string) bool {
	for _, v := range values {
		if v == wanted {
			return true
		}
	}
	return false
}
func containsAll(values, wanted []string) bool {
	for _, value := range wanted {
		if !contains(values, value) {
			return false
		}
	}
	return len(wanted) > 0
}
func validProtocolEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if !(r == '_' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func firstSuffix(value, prefix string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + value
}

func smokePathsOverlap(left, right string) bool {
	left = filepath.ToSlash(filepath.Clean(left))
	right = filepath.ToSlash(filepath.Clean(right))
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func canonicalDirectory(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("not a directory")
	}
	return resolved, nil
}
func canonicalDirectoryInside(root, path string) (string, error) {
	resolved, err := canonicalDirectory(path)
	if err != nil {
		return "", err
	}
	if !inside(root, resolved) {
		return "", fmt.Errorf("path escapes root")
	}
	return resolved, nil
}
func canonicalFileInside(root, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if !inside(root, resolved) {
		return "", fmt.Errorf("path escapes root")
	}
	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", fmt.Errorf("not a file")
	}
	return resolved, nil
}

func smokeEnvironment(settings SmokeSettings, manifest smokeManifest, getenv func(string) string) []string {
	allowed := map[string]bool{}
	for _, name := range settings.Environment {
		allowed[name] = true
	}
	names := append([]string(nil), settings.Environment...)
	for _, name := range manifest.Environment {
		if allowed[name] && !contains(names, name) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var env []string
	for _, name := range names {
		if value := getenv(name); value != "" {
			env = append(env, name+"="+value)
		}
	}
	return env
}

func smokeTimeout(settings SmokeSettings, manifest smokeManifest, req SmokeRequest) (time.Duration, string) {
	if req.Timeout > 0 {
		return req.Timeout, "request"
	}
	if source := settings.Sources["smoke.run_timeout"]; source == "workspace" || source == "env" {
		return settings.RunTimeout, source
	}
	if manifest.Defaults.Timeout != "" {
		if d, err := time.ParseDuration(manifest.Defaults.Timeout); err == nil && d > 0 && d <= 24*time.Hour {
			return d, "manifest"
		}
	}
	return settings.RunTimeout, firstNonEmptyString(settings.Sources["smoke.run_timeout"], "default")
}
