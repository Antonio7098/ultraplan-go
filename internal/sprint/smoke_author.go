package sprint

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

const maxSmokeAuthoringFiles = 20000

func (s Service) authorSmokeSuite(ctx context.Context, prepared smokePrepared, result *SmokeResult) error {
	if s.runtime == nil {
		return smokeError("smoke_author_runtime", "runtime", "runtime is required to author deep-smoke coverage", "Configure the smoke model/runtime and rerun smoke.", nil)
	}
	harnessBefore, err := smokeHarnessSnapshot(prepared)
	if err != nil {
		return smokeError("smoke_author_snapshot", "authoring", "harness could not be snapshotted before authoring", "Restore a readable bounded smoke harness.", err)
	}
	targetBefore, err := targetIdentity(prepared.Target)
	if err != nil {
		return smokeError("smoke_author_target_identity", "authoring", "target identity could not be captured", "Restore a readable target before smoke authoring.", err)
	}
	projectRoot := filepath.Join(s.root, "projects", prepared.Sprint.Project)
	projectBefore, err := targetIdentity(projectRoot)
	if err != nil {
		return smokeError("smoke_author_workspace_identity", "authoring", "governed project identity could not be captured", "Restore readable governed sprint inputs.", err)
	}

	prompt := s.renderSmokeAuthorPrompt(prepared)
	req := s.runtimeRequest(prompt, map[string]string{"project": prepared.Sprint.Project, "sprint": prepared.Sprint.Slug, "stage": string(StageSmoke), "operation": "author"})
	req.WorkDir = prepared.HarnessRoot
	req.Permissions = "restricted"
	req.Policy.UnsupportedBehavior = "best_effort"
	for _, rel := range prepared.Manifest.Authoring.Paths {
		req.Policy.PathRules = append(req.Policy.PathRules, pruntime.PermissionPathRule{Path: filepath.Join(prepared.HarnessRoot, filepath.FromSlash(rel)), Action: "allow"})
	}
	run, runErr := s.runtime.StartRun(ctx, req)
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
	targetAfter, identityErr := targetIdentity(prepared.Target)
	if identityErr != nil || targetAfter != targetBefore {
		return smokeError("smoke_author_target_mutation", "authoring", "smoke author changed or obscured the product target", "Restore the product target and rerun with harness-only authoring authority.", identityErr)
	}
	projectAfter, identityErr := targetIdentity(projectRoot)
	if identityErr != nil || projectAfter != projectBefore {
		return smokeError("smoke_author_workspace_mutation", "authoring", "smoke author changed or obscured governed project inputs", "Restore governed inputs and rerun with harness-only authoring authority.", identityErr)
	}
	if runErr != nil {
		return smokeError("smoke_author_runtime", "runtime", "smoke authoring runtime failed", "Inspect the bounded runtime diagnostics and rerun authoring.", runErr)
	}
	if strings.TrimSpace(run.RunID) == "" {
		return smokeError("smoke_author_identity", "runtime", "smoke authoring returned no run identity", "Require traceable author runtime evidence and rerun smoke.", nil)
	}
	if ctx.Err() != nil {
		return smokeError("smoke_author_cancelled", "cancellation", "smoke authoring was cancelled", "Rerun smoke when authoring can complete.", ctx.Err())
	}
	return nil
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
	fmt.Fprintln(&b, "\nUltraPlan will reject the run if the product target, governed project inputs, or any harness path outside this list changes. After authoring, UltraPlan independently validates discovery coverage and runs the selected suite.")
	return b.String()
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
