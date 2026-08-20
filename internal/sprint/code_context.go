package sprint

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	pruntime "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

var (
	codeContextHeadingRE = regexp.MustCompile(`(?m)^\s*##\s+(.+?)\s*$`)
	codeContextEntryRE   = regexp.MustCompile(`(?m)^\s*###\s+(.+?)\s*$`)
	codeContextPathRE    = regexp.MustCompile(`(?im)^\s*-?\s*\*\*Path:\*\*\s*` + "`?" + `([^` + "`" + `\r\n]+)` + "`?" + `\s*$`)
	codeContextReasonRE  = regexp.MustCompile(`(?im)^\s*-?\s*\*\*Rationale:\*\*\s*(.+?)\s*$`)
	codeContextLinesRE   = regexp.MustCompile(`(?im)^\s*-?\s*\*\*Lines?:\*\*\s*` + "`?" + `([^` + "`" + `\r\n]+)` + "`?" + `\s*$`)
	codeContextFenceRE   = regexp.MustCompile("(?ms)```([A-Za-z0-9_+.-]+)\\s*\\n(.+?)\\n```")
	codeContextDriveRE   = regexp.MustCompile(`^[A-Za-z]:`)
)

func ValidateCodeContextContent(content string) []ValidationFinding {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return []ValidationFinding{finding("code-context.md", "", "", "empty code context", "the artifact has no content", "Generate the code-context artifact from validated requirements.")}
	}
	if containsPlaceholder(trimmed) || containsCodeContextPlaceholder(trimmed) {
		return []ValidationFinding{finding("code-context.md", "", "", "placeholder content", "the artifact still contains template placeholders", "Replace every placeholder with inspected repository evidence.")}
	}
	required := []string{"Sprint Scope", "Inspected Repository Areas", "Selected Source Excerpts", "Relationships", "Constraints", "Open Questions"}
	headings := map[string]bool{}
	for _, match := range codeContextHeadingRE.FindAllStringSubmatch(trimmed, -1) {
		headings[strings.ToLower(strings.TrimSpace(match[1]))] = true
	}
	var findings []ValidationFinding
	for _, heading := range required {
		if !headings[strings.ToLower(heading)] {
			findings = append(findings, finding(heading, "", "", "missing required section", "the required level-two heading is absent", "Add a ## "+heading+" section with concrete content."))
			continue
		}
		if strings.TrimSpace(sectionBody(trimmed, heading)) == "" {
			findings = append(findings, finding(heading, "", "", "empty required section", "the required section has no content", "Add concrete inspected repository evidence to the section."))
		}
	}
	selected := sectionBody(trimmed, "Selected Source Excerpts")
	entries := codeContextEntries(selected)
	if len(entries) == 0 {
		findings = append(findings, finding("Selected Source Excerpts", "", "", "no selected source excerpts", "at least one level-three selected entry is required", "Add at least one selected entry with path, rationale, and exact source fence."))
	}
	for _, entry := range entries {
		pathMatch := codeContextPathRE.FindStringSubmatch(entry.body)
		if len(pathMatch) != 2 || strings.TrimSpace(pathMatch[1]) == "" {
			findings = append(findings, finding("Selected Source Excerpts", entry.name, "", "missing repository-relative path", "the entry has no Path field", "Add **Path:** with a repository-relative source path."))
		} else if err := validateRepositoryRelativePath(strings.TrimSpace(pathMatch[1])); err != nil {
			findings = append(findings, finding("Selected Source Excerpts", entry.name, strings.TrimSpace(pathMatch[1]), "unsafe source path", err.Error(), "Use a clean repository-relative path that does not escape the target."))
		}
		reason := codeContextReasonRE.FindStringSubmatch(entry.body)
		if len(reason) != 2 || strings.TrimSpace(reason[1]) == "" {
			findings = append(findings, finding("Selected Source Excerpts", entry.name, "", "missing rationale", "the entry does not explain why the excerpt matters", "Add a concrete **Rationale:** value."))
		}
		if lines := codeContextLinesRE.FindStringSubmatch(entry.body); len(lines) == 2 {
			if err := validateLineRange(strings.TrimSpace(lines[1])); err != nil {
				findings = append(findings, finding("Selected Source Excerpts", entry.name, "", "malformed line range", err.Error(), "Use a positive line number or inclusive start-end range."))
			}
		}
		fence := codeContextFenceRE.FindStringSubmatch(entry.body)
		if len(fence) != 3 || strings.TrimSpace(fence[1]) == "" || strings.TrimSpace(fence[2]) == "" {
			findings = append(findings, finding("Selected Source Excerpts", entry.name, "", "missing language-tagged source fence", "the entry needs a non-empty fenced exact source block with a language tag", "Add a language-tagged fenced source excerpt."))
		}
	}
	sortSprintFindings(findings)
	return findings
}

func containsCodeContextPlaceholder(content string) bool {
	lower := strings.ToLower(content)
	for _, placeholder := range []string{
		"path/to/file.go",
		"symbolname",
		"describe the requirements-driven implementation scope",
		"describe the packages, commands, boundaries, configuration, and tests inspected",
		"explain why this exact source is relevant",
		"// exact source excerpt",
		"describe how the selected code collaborates",
		"record implementation constraints and invariants",
		"record remaining source-level questions",
	} {
		if strings.Contains(lower, placeholder) {
			return true
		}
	}
	return false
}

type codeContextEntry struct{ name, body string }

func codeContextEntries(section string) []codeContextEntry {
	matches := codeContextEntryRE.FindAllStringSubmatchIndex(section, -1)
	out := make([]codeContextEntry, 0, len(matches))
	for i, match := range matches {
		end := len(section)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		out = append(out, codeContextEntry{name: strings.TrimSpace(section[match[2]:match[3]]), body: section[match[1]:end]})
	}
	return out
}

func sectionBody(content, heading string) string {
	matches := codeContextHeadingRE.FindAllStringSubmatchIndex(content, -1)
	for i, match := range matches {
		if !strings.EqualFold(strings.TrimSpace(content[match[2]:match[3]]), heading) {
			continue
		}
		end := len(content)
		if i+1 < len(matches) {
			end = matches[i+1][0]
		}
		return content[match[1]:end]
	}
	return ""
}

func validateRepositoryRelativePath(value string) error {
	value = strings.Trim(strings.TrimSpace(value), "`")
	if value == "" || filepath.IsAbs(value) || strings.Contains(value, `\`) || codeContextDriveRE.MatchString(value) {
		return fmt.Errorf("path must be non-empty and relative")
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("path escapes or does not identify a source file")
	}
	return nil
}

func validateLineRange(value string) error {
	parts := strings.Split(strings.Trim(strings.TrimSpace(value), "`"), "-")
	if len(parts) < 1 || len(parts) > 2 {
		return fmt.Errorf("range %q must be N or N-M", value)
	}
	start, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || start < 1 {
		return fmt.Errorf("range %q has an invalid start", value)
	}
	if len(parts) == 2 {
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || end < start {
			return fmt.Errorf("range %q has an invalid end", value)
		}
	}
	return nil
}

func (s Service) PromptCodeContext(projectRef, sprintRef string) (PromptPreview, error) {
	sp, inputs, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return PromptPreview{}, err
	}
	if findings := ValidateRequirementsContent(inputs.Requirements); len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("code-context prerequisites failed validation")
	}
	target, findings := s.resolveCodeContextTarget(inputs.ProjectIndex)
	if len(findings) > 0 {
		return PromptPreview{}, fmt.Errorf("code-context target resolution failed: %s", formatValidationFindings(findings))
	}
	return RenderCodeContextPrompt(s.root, sp, inputs.Requirements, target), nil
}

func (s Service) ValidateCodeContext(projectRef, sprintRef string) (ValidationResult, error) {
	sp, _, _, err := s.resolveSprintInputs(projectRef, sprintRef)
	if err != nil {
		return ValidationResult{}, err
	}
	path := mustArtifactPath(s.root, sp, StageCodeContext)
	data, readErr := s.store.ReadArtifact(sp, StageCodeContext)
	var findings []ValidationFinding
	if readErr != nil {
		findings = []ValidationFinding{finding("code-context.md", "", workspace.Rel(s.root, path), "missing code context", readErr.Error(), "Run the code-context stage after requirements validate.")}
	} else {
		findings = ValidateCodeContextContent(data)
	}
	return ValidationResult{Project: sp.Project, Sprint: sp.Slug, Artifact: workspace.Rel(s.root, path), Findings: findings}, nil
}

func (s Service) codeContextPrerequisite(sp Sprint) ([]ValidationFinding, error) {
	validation, err := s.ValidateCodeContext(sp.Project, sp.Slug)
	if err != nil {
		return nil, err
	}
	if !validation.Valid() {
		return validation.Findings, fmt.Errorf("code-context prerequisite failed validation")
	}
	state, err := LoadFlowState(s.root, sp)
	if err != nil {
		return validation.Findings, fmt.Errorf("code-context prerequisite has no successful persisted outcome: %w", err)
	}
	for _, stage := range state.Stages {
		if stage.Stage == StageCodeContext && stage.Status == StatusComplete {
			return nil, nil
		}
	}
	return validation.Findings, fmt.Errorf("code-context prerequisite has not completed successfully")
}

func (s Service) FlowCodeContext(ctx context.Context, projectRef, sprintRef string, req FlowRequest) (FlowResult, error) {
	if req.To != StageCodeContext {
		return FlowResult{}, fmt.Errorf("unsupported code-context flow target %q", req.To)
	}
	sp, inputs, _, err := s.resolveSprintInputsForFlow(projectRef, sprintRef, !req.DryRun)
	if err != nil {
		return FlowResult{}, err
	}
	now := s.now().UTC()
	if findings := ValidateRequirementsContent(inputs.Requirements); len(findings) > 0 {
		err := fmt.Errorf("code-context prerequisites failed validation")
		return s.failCodeContext(sp, req, now, pruntime.Result{}, findings, err)
	}
	target, findings := s.resolveCodeContextTarget(inputs.ProjectIndex)
	if len(findings) > 0 {
		err := fmt.Errorf("code-context target resolution failed")
		return s.failCodeContext(sp, req, now, pruntime.Result{}, findings, err)
	}
	prompt := RenderCodeContextPrompt(s.root, sp, inputs.Requirements, target)
	if req.DryRun {
		return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: true, Message: prompt.Prompt}, nil
	}
	if s.runtime == nil {
		return s.failCodeContext(sp, req, now, pruntime.Result{}, nil, fmt.Errorf("runtime is required for code-context flow"))
	}
	candidate, err := os.CreateTemp(sp.Path, ".code-context.*.candidate.md")
	if err != nil {
		return s.failCodeContext(sp, req, now, pruntime.Result{}, nil, fmt.Errorf("create code-context candidate: %w", err))
	}
	candidatePath := candidate.Name()
	if err := candidate.Close(); err != nil {
		_ = os.Remove(candidatePath)
		return s.failCodeContext(sp, req, now, pruntime.Result{}, nil, fmt.Errorf("close code-context candidate: %w", err))
	}
	defer os.Remove(candidatePath)
	runtimeReq := s.runtimeRequest(prompt.Prompt, map[string]string{"project": sp.Project, "sprint": sp.Slug, "stage": string(StageCodeContext), "output_path": ArtifactRelPath(sp, StageCodeContext), "candidate_path": candidatePath, "target_source": target.Source})
	if strings.TrimSpace(req.ModelOverride) != "" {
		runtimeReq.Provider, runtimeReq.Model = splitProviderModel(strings.TrimSpace(req.ModelOverride))
		runtimeReq.Metadata["model_source"] = "command override"
	} else {
		runtimeReq.Metadata["model_source"] = "effective configuration"
	}
	if strings.TrimSpace(req.VariantOverride) != "" {
		runtimeReq.Metadata["variant"] = strings.TrimSpace(req.VariantOverride)
		runtimeReq.Metadata["reasoning_effort"] = strings.TrimSpace(req.VariantOverride)
		runtimeReq.Metadata["variant_source"] = "command override"
	} else {
		runtimeReq.Metadata["variant_source"] = "effective configuration"
	}
	runtimeReq.WorkDir = target.Path
	runtimeReq.Sandbox = "read_only"
	runtimeReq.Permissions = "restricted"
	runtimeReq.Policy = pruntime.PermissionPolicy{Default: "deny", Tools: map[string]string{"read": "allow", "glob": "allow", "search": "allow", "list": "allow"}}
	runtimeReq.RequireCaps = appendUniqueString(runtimeReq.RequireCaps, "permissions")
	result, runErr := s.runtime.StartRun(ctx, runtimeReq)
	if runErr != nil {
		return s.failCodeContext(sp, req, now, result, nil, runErr)
	}
	if err := ctx.Err(); err != nil {
		return s.failCodeContext(sp, req, now, result, nil, err)
	}
	if result.Permissions.UnsupportedCount > 0 {
		return s.failCodeContext(sp, req, now, result, nil, fmt.Errorf("runtime cannot enforce required code-context read-only permission policy"))
	}
	if status := strings.ToLower(strings.TrimSpace(result.Status)); status != "" && status != "success" && status != "complete" && status != "completed" {
		return s.failCodeContext(sp, req, now, result, nil, fmt.Errorf("code-context runtime ended with status %q", result.Status))
	}
	content := strings.TrimSpace(result.TerminalOutput)
	if content == "" {
		content = strings.TrimSpace(runtimeEventContent(result.Events))
	}
	if content != "" {
		if err := os.WriteFile(candidatePath, []byte(content+"\n"), 0o644); err != nil {
			return s.failCodeContext(sp, req, now, result, nil, fmt.Errorf("write code-context candidate: %w", err))
		}
	}
	data, readErr := os.ReadFile(candidatePath)
	if readErr != nil || len(strings.TrimSpace(string(data))) == 0 {
		if readErr == nil {
			readErr = errors.New("runtime returned no code-context output")
		}
		return s.failCodeContext(sp, req, now, result, nil, readErr)
	}
	findings = ValidateCodeContextContent(string(data))
	if len(findings) > 0 {
		return s.failCodeContext(sp, req, now, result, findings, fmt.Errorf("generated code-context.md failed validation"))
	}
	if err := os.Chmod(candidatePath, 0o644); err != nil {
		return s.failCodeContext(sp, req, now, result, nil, fmt.Errorf("set code-context candidate permissions: %w", err))
	}
	stages := flowCodeContextSuccessStages(sp, now)
	if err := s.promoteCodeContext(ctx, sp, candidatePath, NewFlowState(sp, stages, now)); err != nil {
		return s.failCodeContext(sp, req, now, result, nil, err)
	}
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, Runtime: result, Stages: stages, Message: "code-context complete"}, nil
}

func (s Service) resolveCodeContextTarget(projectIndex string) (ExecuteTargetRef, []ValidationFinding) {
	if s.codeContextTarget != nil {
		return s.codeContextTarget(projectIndex)
	}
	return ResolveExecuteTarget(projectIndex)
}

func (s Service) failCodeContext(sp Sprint, req FlowRequest, now time.Time, result pruntime.Result, findings []ValidationFinding, err error) (FlowResult, error) {
	stages := s.flowFailedStages(sp, StageCodeContext, err, now)
	for i := range stages {
		if stages[i].Stage == StageCodeContext {
			stages[i].LatestOutcome = codeContextLatestOutcome(result, err)
			break
		}
	}
	if !req.DryRun {
		if saveErr := SaveFlowState(s.root, sp, NewFlowState(sp, stages, now)); saveErr != nil {
			err = errors.Join(err, fmt.Errorf("persist failed code-context outcome: %w", saveErr))
		}
	}
	return FlowResult{Project: sp.Project, Sprint: sp.Slug, To: req.To, DryRun: req.DryRun, Runtime: result, Stages: stages, Findings: findings}, err
}

func codeContextLatestOutcome(result pruntime.Result, err error) string {
	if result.Cleanup.Failed || (result.Cleanup.Attempted && !result.Cleanup.Completed) {
		return "cleanup_uncertain"
	}
	status := strings.ToLower(strings.TrimSpace(result.Status))
	if errors.Is(err, context.Canceled) || status == "cancelled" || status == "canceled" {
		return "cancelled"
	}
	if status == "interrupted" {
		return "interrupted"
	}
	return "failed"
}

func (s Service) promoteCodeContext(ctx context.Context, sp Sprint, candidatePath string, state FlowState) error {
	finalPath, err := ArtifactPath(s.root, sp, StageCodeContext)
	if err != nil {
		return err
	}
	old, oldErr := os.ReadFile(finalPath)
	oldExists := oldErr == nil
	if oldErr != nil && !os.IsNotExist(oldErr) {
		return fmt.Errorf("read prior code-context artifact: %w", oldErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(candidatePath, finalPath); err != nil {
		return fmt.Errorf("promote code-context artifact: %w", err)
	}
	persistErr := ctx.Err()
	if persistErr == nil {
		persistErr = SaveFlowState(s.root, sp, state)
	}
	if persistErr != nil {
		if oldExists {
			restore, restoreErr := os.CreateTemp(sp.Path, ".code-context.*.restore.md")
			if restoreErr == nil {
				_, restoreErr = restore.Write(old)
				if restoreErr == nil {
					restoreErr = restore.Sync()
				}
				if closeErr := restore.Close(); restoreErr == nil {
					restoreErr = closeErr
				}
				if restoreErr == nil {
					restoreErr = os.Rename(restore.Name(), finalPath)
				}
				if restoreErr != nil {
					_ = os.Remove(restore.Name())
				}
			}
			if restoreErr != nil {
				return fmt.Errorf("persist code-context state: %v; restore prior artifact: %w", persistErr, restoreErr)
			}
		} else {
			_ = os.Remove(finalPath)
		}
		syncDir(sp.Path)
		return fmt.Errorf("persist code-context state: %w", persistErr)
	}
	syncDir(sp.Path)
	return nil
}

func appendUniqueString(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func runtimeEventContent(events []pruntime.Event) string {
	for i := len(events) - 1; i >= 0; i-- {
		for _, key := range []string{"content", "text", "output"} {
			if value, ok := events[i].Payload[key].(string); ok && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}
