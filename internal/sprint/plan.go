package sprint

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

type PlanManifest struct {
	ProjectSlug        string
	SprintSlug         string
	SprintRoot         string
	RequirementsPath   string
	SprintIndexPath    string
	HandbookPath       string
	ReasoningPath      string
	OutputPath         string
	ReasoningTemplates []ReasoningTemplateEntry
	DecisionNames      []string
	EvidencePhrases    []string
}

func BuildPlanManifest(root string, sp Sprint, inputs PlanningInputs, catalogSprintIndex string, reasoning string) (PlanManifest, []ValidationFinding) {
	_ = root
	index, findings := ParseSprintIndex(catalogSprintIndex)
	manifest := PlanManifest{
		ProjectSlug:      sp.Project,
		SprintSlug:       sp.Slug,
		SprintRoot:       filepath.ToSlash(filepath.Join("projects", sp.Project, "sprints", sp.Slug)),
		RequirementsPath: ArtifactRelPath(sp, StageRequirements),
		SprintIndexPath:  ArtifactRelPath(sp, StageSprintIndex),
		HandbookPath:     ArtifactRelPath(sp, StageTechnicalHandbook),
		ReasoningPath:    ArtifactRelPath(sp, StageReasoning),
		OutputPath:       ArtifactRelPath(sp, StagePlan),
		DecisionNames:    extractReasoningDecisions(reasoning),
		EvidencePhrases:  extractReasoningEvidence(reasoning),
	}
	for _, item := range index.ReasoningTemplates {
		manifest.ReasoningTemplates = append(manifest.ReasoningTemplates, ReasoningTemplateEntry{Name: item.Name, OutputPath: item.Path, Why: item.Why})
	}
	sortSprintFindings(findings)
	return manifest, findings
}

func ValidatePlanContent(content string, manifest PlanManifest) []ValidationFinding {
	var findings []ValidationFinding
	if strings.TrimSpace(content) == "" {
		return []ValidationFinding{finding("plan.md", "", manifest.OutputPath, "empty plan", "file has no content", "Generate or write the required plan sections.")}
	}
	if containsPlaceholder(content) {
		findings = append(findings, finding("plan.md", "", manifest.OutputPath, "placeholder content", "file still contains placeholder markers", "Replace placeholders with concrete sprint plan content."))
	}
	sections := markdownSections(content)
	required := []string{"Decisions To Execute", "Tasks", "Evidence Checklist", "Risks And Blockers", "Completion Criteria"}
	for _, section := range required {
		if strings.TrimSpace(sections[section]) == "" {
			findings = append(findings, finding(section, "", manifest.OutputPath, "missing required section", "section was not found or has no content", "Add the required plan section."))
		}
	}
	if !strings.Contains(content, manifest.ReasoningPath) && !strings.Contains(content, "reasoning.md") {
		findings = append(findings, finding("Reasoning Source", "", manifest.OutputPath, "missing reasoning reference", "plan does not cite reasoning.md", "Reference reasoning.md as the source of decisions."))
	}
	decisionSection := sections["Decisions To Execute"]
	for _, decision := range manifest.DecisionNames {
		if decision != "" && !strings.Contains(decisionSection, decision) && !strings.Contains(content, decision) {
			findings = append(findings, finding("Decisions To Execute", decision, manifest.OutputPath, "missing reasoning decision trace", "plan does not trace a final reasoning decision", "List each final reasoning decision that the plan executes."))
		}
	}
	taskSection := sections["Tasks"]
	if !strings.Contains(taskSection, "- [ ]") && !strings.Contains(taskSection, "- [x]") && !strings.Contains(taskSection, "- [X]") && !strings.Contains(taskSection, "- [/]") {
		findings = append(findings, finding("Tasks", "", manifest.OutputPath, "missing task checklist", "tasks section has no Markdown checkbox items", "Add executable sprint task checkboxes."))
	}
	if len(manifest.DecisionNames) > 0 && containsTaskWithoutTrace(taskSection) {
		findings = append(findings, finding("Tasks", "", manifest.OutputPath, "untraced task", "a task does not cite a decision or acceptance criterion", "Trace each implementation task to reasoning decisions or acceptance evidence."))
	}
	evidenceSection := sections["Evidence Checklist"]
	if !strings.Contains(evidenceSection, "- [ ]") && !strings.Contains(evidenceSection, "- [x]") && !strings.Contains(evidenceSection, "- [X]") {
		findings = append(findings, finding("Evidence Checklist", "", manifest.OutputPath, "missing evidence checklist", "evidence section has no Markdown checkbox items", "Add explicit verification and evidence checkboxes."))
	}
	forbidden := forbiddenPlanStageContent(content)
	for _, phrase := range forbidden {
		findings = append(findings, finding("Deferred Scope", "", manifest.OutputPath, "forbidden deferred-stage behavior", phrase, "Keep implementation execution, smoke, review automation, issues, and Git mutation out of current Phase 2 CLI behavior."))
	}
	sortSprintFindings(findings)
	return findings
}

func extractReasoningDecisions(content string) []string {
	re := regexp.MustCompile(`(?m)^#{2,3}\s+Decision\s+\d+:\s*(.+?)\s*$`)
	matches := re.FindAllStringSubmatch(content, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, strings.TrimSpace(match[1]))
	}
	return out
}

func extractReasoningEvidence(content string) []string {
	sections := markdownSections(content)
	evidence := sections["Expected Evidence"]
	var out []string
	for _, line := range strings.Split(evidence, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "-| "))
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.Contains(strings.ToLower(line), "test") || strings.Contains(strings.ToLower(line), "review") || strings.Contains(strings.ToLower(line), "flow") {
			out = append(out, line)
		}
	}
	return out
}

func containsTaskWithoutTrace(section string) bool {
	var taskBlock []string
	flush := func() bool {
		if len(taskBlock) == 0 {
			return false
		}
		lower := strings.ToLower(strings.Join(taskBlock, "\n"))
		return !strings.Contains(lower, "decision") &&
			!strings.Contains(lower, "ac-") &&
			!strings.Contains(lower, "evidence") &&
			!strings.Contains(lower, "verification") &&
			!strings.Contains(lower, "executes:")
	}
	for _, line := range strings.Split(section, "\n") {
		if isTopLevelTaskCheckbox(line) {
			if flush() {
				return true
			}
			taskBlock = []string{line}
			continue
		}
		if len(taskBlock) > 0 {
			taskBlock = append(taskBlock, line)
		}
	}
	return flush()
}

func isTopLevelTaskCheckbox(line string) bool {
	if strings.TrimLeft(line, " \t") != line {
		return false
	}
	trimmed := strings.TrimSpace(line)
	if len(trimmed) < len("- [ ]") {
		return false
	}
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "- [ ]") || strings.HasPrefix(lower, "- [x]") || strings.HasPrefix(lower, "- [/]")
}

func forbiddenPlanStageContent(content string) []string {
	lower := strings.ToLower(content)
	patterns := []string{
		"flow --to implementation",
		"flow --to execute",
		"flow --to smoke",
		"flow --to review",
		"flow --to issues",
		"generate .run-state.json",
		"create .run-state.json",
		"write .run-state.json",
		"git commit",
		"git push",
	}
	var out []string
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			out = append(out, fmt.Sprintf("contains %q", pattern))
		}
	}
	return out
}
