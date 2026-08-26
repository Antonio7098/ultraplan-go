package sprint

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

const maxSmokeAuthoringFiles = 20000

func (s Service) authorSmokeSuite(ctx context.Context, prepared smokePrepared, result *SmokeResult) error {
	if s.runtime == nil {
		return smokeError("smoke_author_runtime", "runtime", "runtime is required to author deep-smoke coverage", "Configure the smoke model/runtime and rerun smoke.", nil)
	}
	inputs, err := s.store.ReadPlanningInputs(prepared.Sprint)
	if err != nil {
		return smokeError("smoke_author_context", "authoring", "shared sprint context could not be loaded", "Restore the governed planning inputs and rerun smoke.", err)
	}
	sharedPrefix, err := s.prepareSharedPromptContext(ctx, prepared.Sprint, inputs, true)
	if err != nil {
		return smokeError("smoke_author_context", "authoring", "shared sprint context could not be prepared", "Repair the selected source reference or prompt budget failure and rerun smoke.", err)
	}
	harnessBefore, err := smokeHarnessSnapshot(prepared)
	if err != nil {
		return smokeError("smoke_author_snapshot", "authoring", "harness could not be snapshotted before authoring", "Restore a readable bounded smoke harness.", err)
	}
	projectRoot := filepath.Join(s.root, "projects", prepared.Sprint.Project)
	var targetBefore, projectBefore string
	var targetFilesBefore, projectFilesBefore map[string]string
	if strictSmokeAuthorProtectedSnapshots {
		targetBefore, err = targetIdentity(prepared.Target)
		if err != nil {
			return smokeError("smoke_author_target_identity", "authoring", "target identity could not be captured", "Restore a readable target before smoke authoring.", err)
		}
		targetFilesBefore = smokeDiagnosticTargetSnapshot(prepared.Target)
		projectBefore, err = targetIdentity(projectRoot)
		if err != nil {
			return smokeError("smoke_author_workspace_identity", "authoring", "governed project identity could not be captured", "Restore readable governed sprint inputs.", err)
		}
		projectFilesBefore = smokeDiagnosticTargetSnapshot(projectRoot)
	}

	prompt, err := composeStagePromptChecked(sharedPrefix, s.renderSmokeAuthorPrompt(prepared))
	if err != nil {
		return smokeError("smoke_author_context", "authoring", "smoke author prompt could not be composed", "Repair the governed smoke inputs and retry authoring.", err)
	}
	req := s.runtimeRequest(prompt, map[string]string{"project": prepared.Sprint.Project, "sprint": prepared.Sprint.Slug, "stage": string(StageSmoke), "operation": "author"})
	req.WorkDir = prepared.HarnessRoot
	req.Permissions = "restricted"
	req.RequireCaps = appendUnique(req.RequireCaps, "permissions")
	for _, rel := range prepared.Manifest.Authoring.Paths {
		req.Policy.PathRules = append(req.Policy.PathRules, pruntime.PermissionPathRule{Path: filepath.Join(prepared.HarnessRoot, filepath.FromSlash(rel)), Action: "allow"})
	}
	var targetWrite, projectWrite atomic.Bool
	if strictSmokeAuthorProtectedSnapshots {
		req.OnEvent = func(event pruntime.Event) {
			if smokeAuthorAttributedProtectedWrite([]pruntime.Event{event}, prepared.Target) {
				targetWrite.Store(true)
			}
			if smokeAuthorAttributedProtectedWrite([]pruntime.Event{event}, projectRoot) {
				projectWrite.Store(true)
			}
		}
	}
	run, runErr := s.startPlanningStageRun(ctx, prepared.Sprint, StageSmoke, req)
	result.AuthorRunID = run.RunID
	result.AuthorModel = smokeAuthorModel(req)

	harnessAfter, snapshotErr := smokeHarnessSnapshot(prepared)
	if snapshotErr != nil {
		return smokeError("smoke_author_snapshot", "authoring", "harness could not be snapshotted after authoring", "Inspect the authoring run and restore a readable harness.", snapshotErr)
	}
	changed := changedSmokeHarnessPaths(harnessBefore, harnessAfter)
	result.AuthorChangedPaths = changed
	for _, rel := range changed {
		if !smokeAuthorPathAllowed(rel, prepared.Manifest.Authoring.Paths) {
			return smokeError("smoke_author_scope", "authoring", "smoke author modified a path outside the manifest allowlist: "+rel, "Revert the out-of-scope harness change and tighten the authoring prompt/policy.", nil)
		}
	}
	if strictSmokeAuthorProtectedSnapshots {
		targetAfter, identityErr := targetIdentity(prepared.Target)
		if identityErr != nil || targetAfter != targetBefore {
			changedPaths := changedSmokeHarnessPaths(targetFilesBefore, smokeDiagnosticTargetSnapshot(prepared.Target))
			if targetWrite.Load() || smokeAuthorAttributedProtectedWrite(run.Events, prepared.Target) {
				return smokeError("smoke_author_target_mutation", "authoring", "smoke author made a write-capable tool call against the product target", "Restore the product target and rerun with harness-only authoring authority.", identityErr)
			}
			result.Diagnostics = append(result.Diagnostics, smokeConcurrentChangeDiagnostic("product target", changedPaths))
		}
		projectAfter, identityErr := targetIdentity(projectRoot)
		if identityErr != nil || projectAfter != projectBefore {
			changedPaths := changedSmokeHarnessPaths(projectFilesBefore, smokeDiagnosticTargetSnapshot(projectRoot))
			if projectWrite.Load() || smokeAuthorAttributedProtectedWrite(run.Events, projectRoot) {
				return smokeError("smoke_author_workspace_mutation", "authoring", "smoke author made a write-capable tool call against governed project inputs", "Restore governed inputs and rerun with harness-only authoring authority.", identityErr)
			}
			result.Diagnostics = append(result.Diagnostics, smokeConcurrentChangeDiagnostic("governed project inputs", changedPaths))
		}
	}
	if runErr != nil {
		return smokeError("smoke_author_runtime", "runtime", "smoke authoring runtime failed", "Inspect the bounded runtime diagnostics and rerun authoring.", runErr)
	}
	if run.Permissions.UnsupportedCount > 0 {
		return smokeError("smoke_author_permissions", "permissions", "smoke authoring permission enforcement was unsupported", "Use a runtime that can enforce the restricted harness authoring boundary.", nil)
	}
	if strings.TrimSpace(run.RunID) == "" {
		return smokeError("smoke_author_identity", "runtime", "smoke authoring returned no run identity", "Require traceable author runtime evidence and rerun smoke.", nil)
	}
	if ctx.Err() != nil {
		return smokeError("smoke_author_cancelled", "cancellation", "smoke authoring was cancelled", "Rerun smoke when authoring can complete.", ctx.Err())
	}
	_ = s.cleanupPlanningStageSessions(ctx, prepared.Sprint, StageSmoke, run)
	return nil
}

func smokeConcurrentChangeDiagnostic(scope string, paths []string) string {
	detail := "none identified"
	if len(paths) > 0 {
		const maxPaths = 20
		shown := paths
		if len(shown) > maxPaths {
			shown = shown[:maxPaths]
		}
		detail = strings.Join(shown, ", ")
		if len(paths) > len(shown) {
			detail += fmt.Sprintf(" (+%d more)", len(paths)-len(shown))
		}
	}
	return fmt.Sprintf("concurrent_target_change: %s changed during smoke authoring without an observed OpenCode protected-path write; changed paths: %s", scope, detail)
}

func smokeAuthorAttributedProtectedWrite(events []pruntime.Event, protectedRoot string) bool {
	root := filepath.ToSlash(filepath.Clean(protectedRoot))
	for _, event := range events {
		if event.Kind != "tool" {
			continue
		}
		data, err := json.Marshal(event.Payload)
		if err != nil {
			continue
		}
		text := strings.ToLower(filepath.ToSlash(string(data)))
		if !strings.Contains(text, strings.ToLower(root)) {
			continue
		}
		for _, marker := range []string{
			`"tool":"write`, `"tool":"edit`, `"tool":"patch`,
			`"name":"write`, `"name":"edit`, `"name":"patch`,
			"apply_patch", "write_file", "writefile", "filesystem_write",
		} {
			if strings.Contains(text, marker) {
				return true
			}
		}
		if strings.Contains(text, `"command"`) {
			for _, marker := range []string{
				"apply_patch", " > ", ">>", "sed -i", "perl -pi", "gofmt -w",
				" tee ", " touch ", " chmod ", " rm ", " mv ", " cp ",
			} {
				if strings.Contains(text, marker) {
					return true
				}
			}
		}
	}
	return false
}

// smokeDiagnosticTargetSnapshot is diagnostic only. Failure to enumerate a
// target never changes the authoring verdict; the authoritative identity check
// still reports that drift occurred.
func smokeDiagnosticTargetSnapshot(root string) map[string]string {
	out := map[string]string{}
	root, err := filepath.Abs(root)
	if err != nil {
		return out
	}
	listed, err := exec.Command("git", "-C", root, "ls-files", "-co", "--exclude-standard", "-z").Output()
	if err != nil {
		return out
	}
	for _, rel := range strings.Split(string(listed), "\x00") {
		if rel == "" {
			continue
		}
		rel = filepath.ToSlash(filepath.Clean(rel))
		path := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(path)
		if err != nil {
			out[rel] = "missing"
			continue
		}
		if !info.Mode().IsRegular() || info.Size() > 64<<20 {
			out[rel] = fmt.Sprintf("mode:%s:size:%d", info.Mode(), info.Size())
			continue
		}
		digest, err := hashFile(path)
		if err == nil {
			out[rel] = digest
		}
	}
	return out
}

func (s Service) renderSmokeAuthorPrompt(prepared smokePrepared) string {
	body, source := sprintPromptTemplate(s.root, "prompts/smoke.md")
	var b strings.Builder
	b.WriteString(strings.TrimSpace(body))
	fmt.Fprintf(&b, "\n\n---\n\n## UltraPlan Smoke Authoring Manifest\n\nPrompt source: `%s`\n", source)
	fmt.Fprintf(&b, "Project: `%s`\nSprint: `%s`\nPlanning workspace (read-only): `%s`\n", prepared.Sprint.Project, prepared.Sprint.Slug, s.root)
	fmt.Fprintf(&b, "Sprint root (read-only): `%s`\nProduct target (read-only): `%s`\n", prepared.Sprint.Path, prepared.Target)
	fmt.Fprintf(&b, "Smoke harness (only writable root): `%s`\nHarness manifest: `%s`\n", prepared.HarnessRoot, prepared.ManifestPath)
	fmt.Fprintln(&b, "\nRequired governed inputs:")
	for _, rel := range []string{"requirements.md", "sprint-index.md", "technical-handbook.md", "reasoning.md", "plan.md", "execute.md", "review.md", ".run-state.json"} {
		fmt.Fprintf(&b, "- `%s`\n", filepath.Join(prepared.Sprint.Path, rel))
	}
	fmt.Fprintln(&b, "\nWritable harness paths:")
	for _, rel := range prepared.Manifest.Authoring.Paths {
		fmt.Fprintf(&b, "- `%s`\n", filepath.Join(prepared.HarnessRoot, filepath.FromSlash(rel)))
	}
	fmt.Fprintln(&b, `
The writable-path list above is exhaustive, not illustrative:

- Use the existing-coverage fast path before doing broad analysis or making any
  edit: run only the harness discovery command and inspect the requested
  sprint's existing suite, test identities, coverage IDs, and mapping. Also
  inspect the implementation of every selected test and compare the behavior
  it actually exercises with the governed acceptance criterion it declares.
  Coverage IDs and a complete mapping are routing metadata, not evidence. A
  test may claim a coverage ID only when its command crosses the real boundary
  that deterministic product tests cannot prove. Invoking or wrapping unit,
  package, integration, fake-runtime, or other normal product tests is not a
  deep-smoke probe, even when it executes against the real source tree. A
  broad test-name regex is insufficient unless the harness also proves that
  the intended tests matched and ran. If a governed input records a real
  runtime, provider, browser, process, recovery, or filesystem probe as
  blocked, a local regression command cannot convert that criterion to passed;
  preserve the exact blocker or author the missing real-boundary probe.
  Build a concise criterion-to-probe audit for the existing suite before using
  the fast path. The union of declared coverage IDs alone must never justify a
  no-change success.
  Also
  inspect the run adapter statically: every failed or errored result must be
  associated with an open issue returned in the protocol response, including
  ID, status, path, test ID, severity, title, observed summary, falsifiable
  theory, supporting evidence, and next action. An issue file written to disk
  is not sufficient if the response omits it; a constant empty issues array is
  invalid. Each returned issue must use this exact JSON shape and value types:
  {"id":"...","status":"open","path":"issues/<id>.md","test_id":"<failed-test-id>","severity":"...","title":"...","summary":"...","theory":"...","evidence":"...","action":"..."}.
  Use the exact snake_case keys shown. Do not substitute camelCase aliases such
  as testId, observedSummary, falsifiableTheory, supportingEvidence, or
  nextAction. Every displayed value must be a non-empty string; in particular,
  evidence is one string, not an array. Only when discovery and this
  failure-to-issue wiring and the criterion-to-probe audit are complete,
  semantically consistent, and aligned with the current governed sprint inputs
  may you make no changes and return success promptly.
  Do not add opportunistic tests or rebuild the acceptance matrix merely
  because authoring was invoked again.
- If existing work is incomplete, resume it narrowly. Repair only concrete
  discovery, mapping, compilation, or governed-coverage gaps needed to make the
  existing sprint suite coherent. Prefer finishing the current suite over
  expanding it. During authoring, use only bounded discovery and static or type
  checks. Do not execute the harness run command, browsers, product builds,
  product test suites, or the authoritative smoke lane; UltraPlan performs
  independent discovery validation and execution after authoring returns.
- Creating a test file is not completion. Register every new or adopted test in
  all harness indexes, suite lists, command branches, and sprint mappings that
  the existing harness convention requires. Before returning, run the manifest's
  discovery command again for this sprint. Confirm that it reports a complete
  sprint mapping, at least one selected test, and every intended coverage ID.
  If discovery reports zero tests, a missing suite, an incomplete mapping, or
  stale coverage, repair the registration and repeat discovery. Do not return
  while authored coverage is present on disk but undiscoverable.
- Before authoring new coverage, inspect the existing harness tests, suites,
  sprint mappings, and related files for work left by an earlier unfinished
  smoke-authoring run for this sprint. Only an existing file already inside a
  writable path may be adopted directly. If its test is relevant, technically
  sound, and compatible with the current governed inputs and harness
  conventions, complete or repair it as needed and include it in the
  appropriate suite and sprint mapping. Do not duplicate equivalent coverage.
  Treat existing files as candidates, not as authoritative: ignore or replace
  stale, unrelated, invalid, or unsafe work. Do not weaken assertions merely to make it pass.
- Every pre-existing path outside the writable-path list is strictly read-only,
  even when it contains useful unfinished test code. Never edit, rename,
  delete, touch, or change permissions on such a path. If its ideas are useful,
  reimplement only the relevant logic in a file inside a writable directory;
  do not adopt or import the out-of-scope file itself.
- A listed directory authorizes files below that directory only. It does not
  authorize a similarly named sibling or the directory's parent.
- A listed file authorizes that exact file only.
- Do not create scratch, debug, probe, backup, generated, or temporary files
  anywhere else in the harness. Put durable test code below an allowed test or
  scenario directory, and use process-managed temporary storage for ephemeral
  experiments.
- Before every write, resolve the destination and confirm it is either an exact
  listed file or a descendant of a listed directory.
- Before finishing, inspect every path changed during this authoring session.
  If this session created an out-of-scope path that did not exist beforehand,
  remove it and relocate the intended content into an allowed path. Never try
  to clean up, remove, or relocate a pre-existing out-of-scope path: leave it
  byte-for-byte and metadata-for-metadata unchanged.

For example, when "src/tests" is listed, "src/tests/probe.ts" is allowed but
"src/test-probe.ts" is forbidden. UltraPlan snapshots the entire harness and
will reject the whole authoring run if even one path outside the list changes;
successful tests do not override that rejection. The product target and
governed project inputs are also read-only. After authoring, UltraPlan
independently validates discovery coverage and runs the selected suite.`)
	inputsToInject := []directPromptInput{
		directSprintArtifactInput(s.root, prepared.Sprint, StageSprintIndex),
		directSprintArtifactInput(s.root, prepared.Sprint, StageTechnicalHandbook),
	}
	inputsToInject = append(inputsToInject, directReasoningDirectoryInputs(s.root, prepared.Sprint)...)
	inputsToInject = append(inputsToInject,
		directSprintArtifactInput(s.root, prepared.Sprint, StageReasoning),
		directSprintArtifactInput(s.root, prepared.Sprint, StagePlan),
		directSprintArtifactInput(s.root, prepared.Sprint, StageExecute),
		directSprintArtifactInput(s.root, prepared.Sprint, StageReview),
		directExecutionHandoffInput(s.root, prepared.Sprint),
	)
	return appendDirectInputPacket(b.String(), inputsToInject)
}

func smokeAuthorModel(req pruntime.Request) string {
	if req.Provider != "" && req.Model != "" {
		return req.Provider + "/" + req.Model
	}
	if req.Model != "" {
		return req.Model
	}
	return "runtime default"
}

func smokeHarnessSnapshot(prepared smokePrepared) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(prepared.HarnessRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(prepared.HarnessRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".git" || rel == "node_modules" || inside(prepared.RunsRoot, path) || inside(prepared.IssuesRoot, path) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(out) >= maxSmokeAuthoringFiles {
			return fmt.Errorf("harness exceeds %d authoring files", maxSmokeAuthoringFiles)
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("authoring snapshot rejects symlink: %s", rel)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > 64<<20 {
			return fmt.Errorf("authoring file exceeds bounded read: %s", rel)
		}
		digest, err := hashFile(path)
		if err != nil {
			return err
		}
		out[rel] = digest
		return nil
	})
	return out, err
}

func changedSmokeHarnessPaths(before, after map[string]string) []string {
	seen := map[string]bool{}
	var changed []string
	for path, digest := range before {
		seen[path] = true
		if after[path] != digest {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if !seen[path] {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	return changed
}

func smokeAuthorPathAllowed(path string, allowed []string) bool {
	path = filepath.ToSlash(filepath.Clean(path))
	for _, candidate := range allowed {
		candidate = filepath.ToSlash(filepath.Clean(candidate))
		if path == candidate || strings.HasPrefix(path, candidate+"/") {
			return true
		}
	}
	return false
}
