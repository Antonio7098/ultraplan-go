package sprint

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ExecutePlanTask struct {
	ID           string
	Name         string
	PlanLine     int
	Checked      bool
	Deferred     bool
	DeferReason  string
	Steps        []string
	Decisions    []string
	Requirements []string
	Evidence     []string
	// Model optionally overrides the runtime model (provider/model) for this
	// task via an inline `<!-- model: provider/model -->` plan annotation.
	Model string
}

func ExtractExecutePlanTasks(content string, manifest PlanManifest) ([]ExecutePlanTask, []ValidationFinding) {
	return extractExecutePlanTasks(content, manifest, false)
}

func extractExecutePlanTasks(content string, manifest PlanManifest, allowChecked bool) ([]ExecutePlanTask, []ValidationFinding) {
	var findings []ValidationFinding
	planFindings := ValidatePlanContent(content, manifest)
	if len(planFindings) > 0 {
		return nil, planFindings
	}
	taskSection := markdownSections(content)["Tasks"]
	if strings.TrimSpace(taskSection) == "" {
		return nil, []ValidationFinding{finding("Tasks", "", manifest.OutputPath, "missing tasks section", "plan has no Tasks section", "Add explicit top-level task checklist entries.")}
	}
	lines := strings.Split(taskSection, "\n")
	var tasks []ExecutePlanTask
	var current *ExecutePlanTask
	flush := func() {
		if current == nil {
			return
		}
		current.Decisions = uniqueSorted(current.Decisions)
		current.Requirements = uniqueSorted(current.Requirements)
		current.Evidence = uniqueSorted(current.Evidence)
		current.Steps = uniqueStable(current.Steps)
		current.ID = deterministicExecuteTaskID(*current)
		tasks = append(tasks, *current)
		current = nil
	}
	baseLine := sectionStartLine(content, "Tasks")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if isTopLevelTaskCheckbox(line) {
			flush()
			title, checked, ok, taskModel := parseTopLevelTaskLine(line)
			deferred := isDeferredTaskLine(line)
			deferReason := deferredTaskReason(line)
			if !ok {
				findings = append(findings, finding("Tasks", "", manifest.OutputPath, "unsupported task syntax", strings.TrimSpace(line), "Use '- [ ] Task N: Name' or '- [ ] **Task N: Name**'."))
				continue
			}
			if checked {
				if !allowChecked {
					findings = append(findings, finding("Tasks", title, manifest.OutputPath, "completed task cannot be executed", "top-level task is already checked", "Leave executable plan tasks unchecked until execute records completion."))
				}
			}
			if deferred {
				if deferReason == "" {
					findings = append(findings, finding("Tasks", title, manifest.OutputPath, "deferred task lacks rationale", "top-level task uses [/] without an inline Deferred reason", "Use '- [/] Task N: Name — Deferred: concrete reason'."))
				}
				if !allowChecked {
					findings = append(findings, finding("Tasks", title, manifest.OutputPath, "deferred task cannot start a new execution", "top-level task is already marked deferred", "Start with [ ] and let execute record the agent's deferral, or resume the matching run state."))
				}
			}
			current = &ExecutePlanTask{Name: title, PlanLine: baseLine + i + 1, Checked: checked, Deferred: deferred, DeferReason: deferReason, Model: taskModel}
			current.Decisions = append(current.Decisions, extractRefs(title, `Decision\s+\d+`)...)
			current.Requirements = append(current.Requirements, extractRefs(title, `(?:REQ-\d+-\d+|AC-\d+)`)...)
			continue
		}
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "- [") {
			if current == nil {
				findings = append(findings, finding("Tasks", "", manifest.OutputPath, "ambiguous checklist item", strings.TrimSpace(line), "Nested checklist items must appear under an explicit top-level Task entry."))
				continue
			}
			item := strings.TrimSpace(stripCheckbox(line))
			current.Steps = append(current.Steps, item)
			lower := strings.ToLower(item)
			if strings.Contains(lower, "evidence") || strings.Contains(lower, "verification") || strings.Contains(lower, "test") || strings.Contains(lower, "check") {
				current.Evidence = append(current.Evidence, item)
			}
			current.Decisions = append(current.Decisions, extractRefs(item, `Decision\s+\d+`)...)
			current.Requirements = append(current.Requirements, extractRefs(item, `(?:REQ-\d+-\d+|AC-\d+)`)...)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, ">") && current != nil {
			current.Decisions = append(current.Decisions, extractRefs(trimmed, `Decision\s+\d+`)...)
			current.Requirements = append(current.Requirements, extractRefs(trimmed, `(?:REQ-\d+-\d+|AC-\d+)`)...)
		}
	}
	flush()
	if len(findings) > 0 {
		sortSprintFindings(findings)
		return nil, findings
	}
	if len(tasks) == 0 {
		return nil, []ValidationFinding{finding("Tasks", "", manifest.OutputPath, "no executable tasks", "no supported unchecked top-level Task checklist entries were found", "Add explicit '- [ ] Task N: ...' entries.")}
	}
	seen := map[string]ExecutePlanTask{}
	for _, task := range tasks {
		if prior, ok := seen[task.ID]; ok {
			findings = append(findings, finding("Tasks", task.Name, manifest.OutputPath, "duplicate task id", fmt.Sprintf("%s duplicates %s", task.Name, prior.Name), "Make task identity fields distinct."))
		}
		seen[task.ID] = task
	}
	sortSprintFindings(findings)
	return tasks, findings
}

func PlanFingerprint(content string) string {
	sum := sha256.Sum256([]byte(normalizeTaskText(content)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ExecuteTasksToRecords(tasks []ExecutePlanTask, now func() time.Time) []ExecuteTaskRecord {
	records := make([]ExecuteTaskRecord, 0, len(tasks))
	current := now()
	for _, task := range tasks {
		records = append(records, ExecuteTaskRecord{
			ID: task.ID,
			Identity: ExecuteTaskIdentity{
				Name:         task.Name,
				PlanLine:     task.PlanLine,
				Decisions:    task.Decisions,
				Requirements: task.Requirements,
				Evidence:     task.Evidence,
			},
			Status:    ExecuteTaskPending,
			Attempts:  0,
			CreatedAt: current,
			UpdatedAt: current,
		})
	}
	return records
}

func deterministicExecuteTaskID(task ExecutePlanTask) string {
	parts := []string{task.Name}
	parts = append(parts, task.Decisions...)
	parts = append(parts, task.Requirements...)
	parts = append(parts, task.Evidence...)
	sum := sha256.Sum256([]byte(normalizeTaskText(strings.Join(parts, "\n"))))
	return "task-" + hex.EncodeToString(sum[:])[:12]
}

// taskModelAnnotationPattern matches an inline per-task runtime model
// annotation, e.g. `- [ ] Task 1: Name <!-- model: provider/model -->`.
var taskModelAnnotationPattern = regexp.MustCompile(`(?i)<!--\s*model:\s*([A-Za-z0-9._~/-]+)\s*-->`)

func parseTopLevelTaskLine(line string) (string, bool, bool, string) {
	trimmed := strings.TrimSpace(line)
	checked := strings.HasPrefix(strings.ToLower(trimmed), "- [x]")
	body := strings.TrimSpace(trimmed[5:])
	body, _ = splitDeferredTaskReason(body)
	model := ""
	if match := taskModelAnnotationPattern.FindStringSubmatchIndex(body); match != nil {
		model = strings.TrimSpace(body[match[2]:match[3]])
		body = strings.TrimSpace(body[:match[0]] + body[match[1]:])
	}
	body = strings.Trim(body, "* ")
	if !regexp.MustCompile(`(?i)^Task\s+\d+\s*:`).MatchString(body) {
		return "", checked, false, model
	}
	body = strings.TrimSpace(strings.Trim(body, "* "))
	return body, checked, true, model
}

func isDeferredTaskLine(line string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "- [/]")
}

func deferredTaskReason(line string) string {
	_, reason := splitDeferredTaskReason(strings.TrimSpace(line)[5:])
	return reason
}

func splitDeferredTaskReason(body string) (string, string) {
	re := regexp.MustCompile(`(?i)\s+(?:—|-)\s+deferred:\s*(.+?)\s*$`)
	match := re.FindStringSubmatchIndex(body)
	if match == nil {
		return body, ""
	}
	return strings.TrimSpace(body[:match[0]]), strings.TrimSpace(body[match[2]:match[3]])
}

func stripCheckbox(line string) string {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) >= 5 && strings.HasPrefix(trimmed, "- [") {
		return strings.TrimSpace(trimmed[5:])
	}
	return trimmed
}

func extractRefs(text, pattern string) []string {
	re := regexp.MustCompile(`(?i)` + pattern)
	matches := re.FindAllString(text, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, strings.TrimSpace(match))
	}
	return out
}

func sectionStartLine(content, section string) int {
	re := regexp.MustCompile(`(?m)^#{2,6}\s+` + regexp.QuoteMeta(section) + `\s*$`)
	loc := re.FindStringIndex(content)
	if loc == nil {
		return 0
	}
	return strings.Count(content[:loc[0]], "\n") + 1
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueStable(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func normalizeTaskText(value string) string {
	fields := strings.Fields(strings.ToLower(value))
	return strings.Join(fields, " ")
}
