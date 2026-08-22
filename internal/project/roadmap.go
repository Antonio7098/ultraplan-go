package project

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type RoadmapSprintStatus string

const (
	RoadmapPlanned   RoadmapSprintStatus = "planned"
	RoadmapActive    RoadmapSprintStatus = "active"
	RoadmapDelivered RoadmapSprintStatus = "delivered"
	RoadmapDropped   RoadmapSprintStatus = "dropped"
)

func (s RoadmapSprintStatus) Valid() bool {
	switch s {
	case RoadmapPlanned, RoadmapActive, RoadmapDelivered, RoadmapDropped:
		return true
	}
	return false
}

type RoadmapSprint struct {
	Number    int
	Title     string
	Slug      string
	Status    RoadmapSprintStatus
	DependsOn []int
	Line      int
	HasGoal   bool
	HasBuild  bool
	HasGate   bool
	Goal      string
	GateItems []string
}

type RoadmapPhase struct {
	Title   string
	Line    int
	Sprints []RoadmapSprint
}

type Roadmap struct {
	Phases  []RoadmapPhase
	Sprints []RoadmapSprint
}

type RoadmapIssue struct {
	Line       int
	Problem    string
	Cause      string
	Suggestion string
}

var roadmapAllowedSubsections = map[string]bool{
	"goal":         true,
	"build":        true,
	"acceptance":   true,
	"release gate": true,
	"exit gate":    true,
	"evidence":     true,
	"uncertainty":  true,
	"deferred":     true,
	"deliverables": true,
	"commands":     true,
	"notes":        true,
}

// roadmapGateSubsections are the subsections that satisfy the acceptance-gate
// requirement; any one of them is sufficient.
var roadmapGateSubsections = map[string]bool{
	"acceptance":   true,
	"release gate": true,
	"exit gate":    true,
}

var roadmapAllowedMetadata = map[string]bool{
	"slug":        true,
	"status":      true,
	"depends on":  true,
	"uncertainty": true,
}

// ParseRoadmap parses the governed roadmap structure. Sprints are H3 sections
// titled "Sprint N: Title" inside H2 phase sections. Each sprint declares its
// sprint directory through a "> Slug:" metadata line, an optional status and
// dependency list, and requires Goal, Build, and Acceptance subsections.
// Content inside fenced code blocks is ignored.
func ParseRoadmap(content string) (Roadmap, []RoadmapIssue) {
	var (
		roadmap Roadmap
		issues  []RoadmapIssue
	)
	lines := strings.Split(content, "\n")
	var currentSprint *RoadmapSprint
	sprintStarted := false
	subsection := ""
	goalClosed := false
	inFence := false

	add := func(line int, problem, cause, suggestion string) {
		issues = append(issues, RoadmapIssue{Line: line, Problem: problem, Cause: cause, Suggestion: suggestion})
	}

	for i, raw := range lines {
		line := raw
		number := i + 1
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" {
			if !inFence && trimmed == "" && subsection == "goal" && currentSprint != nil && currentSprint.Goal != "" {
				goalClosed = true
			}
			continue
		}
		level := headingLevel(line)
		if level == 0 {
			if currentSprint == nil {
				continue
			}
			if sprintStarted {
				collectSprintContent(currentSprint, subsection, goalClosed, trimmed)
				continue
			}
			if strings.HasPrefix(trimmed, ">") {
				parseSprintMetadata(currentSprint, trimmed[1:], number, add)
			}
			continue
		}
		text := strings.TrimSpace(strings.TrimLeft(line, "#"))
		switch level {
		case 2:
			roadmap.Phases = append(roadmap.Phases, RoadmapPhase{Title: text, Line: number})
			currentSprint = nil
		case 3:
			sprintNumber, title, ok := parseSprintHeading(text)
			if !ok {
				currentSprint = nil
				continue
			}
			if len(roadmap.Phases) == 0 {
				add(number, "sprint section outside a phase", fmt.Sprintf("Sprint %d has no preceding '## Phase' section", sprintNumber), "Add a '## Phase N: <title>' section before the sprint.")
				currentSprint = nil
				break
			}
			phase := &roadmap.Phases[len(roadmap.Phases)-1]
			phase.Sprints = append(phase.Sprints, RoadmapSprint{Number: sprintNumber, Title: title, Line: number})
			currentSprint = &phase.Sprints[len(phase.Sprints)-1]
			sprintStarted = false
			subsection = ""
			goalClosed = false
		default:
			if currentSprint == nil {
				break
			}
			if level == 1 {
				currentSprint = nil
				break
			}
			if level != 4 {
				add(number, "unexpected heading inside sprint section", fmt.Sprintf("heading level %d is not allowed inside a sprint", level), "Use '#### <subsection>' headings inside sprints and '### Sprint' for the next sprint.")
				currentSprint = nil
				break
			}
			label := strings.ToLower(text)
			sprintStarted = true
			subsection = label
			if label == "goal" {
				currentSprint.HasGoal = true
			}
			if label == "build" {
				currentSprint.HasBuild = true
			}
			if roadmapGateSubsections[label] {
				currentSprint.HasGate = true
			}
			if !roadmapAllowedSubsections[label] {
				add(number, "unexpected sprint subsection", fmt.Sprintf("'%s' is not a supported sprint subsection", text), "Use Goal, Build, Acceptance, Release Gate, Exit Gate, Evidence, Uncertainty, Deferred, Deliverables, Commands, or Notes.")
			}
		}
	}
	phases := make([]RoadmapPhase, 0, len(roadmap.Phases))
	for _, phase := range roadmap.Phases {
		if len(phase.Sprints) == 0 {
			continue
		}
		phases = append(phases, phase)
	}
	roadmap.Phases = phases
	for _, phase := range roadmap.Phases {
		roadmap.Sprints = append(roadmap.Sprints, phase.Sprints...)
	}
	checkSprints(roadmap.Sprints, add)
	return roadmap, issues
}

func headingLevel(line string) int {
	count := 0
	for count < len(line) && line[count] == '#' {
		count++
	}
	if count > 0 && count <= 6 && count < len(line) && line[count] == ' ' {
		return count
	}
	return 0
}

var sprintHeadingPattern = regexp.MustCompile(`^Sprint (\d+): (.+)$`)

func parseSprintHeading(text string) (int, string, bool) {
	matches := sprintHeadingPattern.FindStringSubmatch(text)
	if matches == nil {
		return 0, "", false
	}
	number, err := strconv.Atoi(matches[1])
	if err != nil || number <= 0 {
		return 0, "", false
	}
	return number, strings.TrimSpace(matches[2]), true
}

func parseSprintMetadata(sprint *RoadmapSprint, text string, line int, add func(int, string, string, string)) {
	trimmed := strings.TrimSpace(text)
	key, value, found := strings.Cut(trimmed, ":")
	if !found {
		add(line, "invalid sprint metadata line", fmt.Sprintf("'%s' is not 'Key: value'", trimmed), "Use '> Slug: <dir>', '> Status:', '> Depends On:', or '> Uncertainty:'.")
		return
	}
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	value = strings.TrimSpace(value)
	if !roadmapAllowedMetadata[normalizedKey] {
		add(line, "unknown sprint metadata field", fmt.Sprintf("'%s' is not a supported metadata key", key), "Use Slug, Status, Depends On, or Uncertainty.")
		return
	}
	switch normalizedKey {
	case "slug":
		sprint.Slug = value
	case "status":
		status := RoadmapSprintStatus(strings.ToLower(value))
		if !status.Valid() {
			add(line, "invalid sprint status", fmt.Sprintf("'%s' is not one of planned, active, delivered, dropped", value), "Use one of planned, active, delivered, dropped.")
			return
		}
		sprint.Status = status
	case "depends on":
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			dependency, err := strconv.Atoi(part)
			if err != nil || dependency <= 0 {
				add(line, "invalid dependency reference", fmt.Sprintf("'%s' is not a sprint number", part), "List comma-separated sprint numbers, e.g. '> Depends On: 1, 3'.")
				continue
			}
			sprint.DependsOn = append(sprint.DependsOn, dependency)
		}
	}
}

// collectSprintContent captures displayable sprint content: the first Goal
// paragraph and acceptance-gate checklist items.
func collectSprintContent(sprint *RoadmapSprint, subsection string, goalClosed bool, text string) {
	switch {
	case subsection == "goal" && !goalClosed:
		if sprint.Goal == "" {
			sprint.Goal = text
			return
		}
		sprint.Goal += " " + text
	case roadmapGateSubsections[subsection] && strings.HasPrefix(text, "-"):
		item := strings.TrimSpace(strings.TrimPrefix(text, "-"))
		item = strings.TrimSpace(item)
		for _, checkbox := range []string{"[ ] ", "[x] ", "[X] "} {
			if strings.HasPrefix(item, checkbox) {
				item = strings.TrimSpace(strings.TrimPrefix(item, checkbox))
				break
			}
		}
		sprint.GateItems = append(sprint.GateItems, item)
	}
}

func checkSprints(sprints []RoadmapSprint, add func(int, string, string, string)) {
	seenNumbers := make(map[int]int)
	seenSlugs := make(map[string]int)
	numbers := make(map[int]bool)
	previous := 0
	for _, sprint := range sprints {
		if first, duplicate := seenNumbers[sprint.Number]; duplicate {
			add(sprint.Line, "duplicate sprint number", fmt.Sprintf("Sprint %d already declared at line %d", sprint.Number, first), "Use unique sprint numbers.")
		} else {
			seenNumbers[sprint.Number] = sprint.Line
		}
		if previous > 0 && sprint.Number < previous {
			add(sprint.Line, "sprint numbers out of order", fmt.Sprintf("Sprint %d follows Sprint %d", sprint.Number, previous), "Order sprint sections by increasing sprint number.")
		}
		previous = sprint.Number
		numbers[sprint.Number] = true
		if sprint.Slug == "" {
			add(sprint.Line, "missing sprint slug", fmt.Sprintf("Sprint %d does not declare a slug", sprint.Number), "Add '> Slug: <sprint-directory>' directly below the sprint heading.")
		} else if first, duplicate := seenSlugs[sprint.Slug]; duplicate {
			add(sprint.Line, "duplicate sprint slug", fmt.Sprintf("'%s' is already used by the sprint at line %d", sprint.Slug, first), "Use one roadmap entry per sprint directory.")
		} else {
			seenSlugs[sprint.Slug] = sprint.Line
		}
		if !sprint.HasGoal {
			add(sprint.Line, "sprint missing goal", fmt.Sprintf("Sprint %d has no '#### Goal' subsection", sprint.Number), "Add a '#### Goal' subsection describing what done means.")
		}
		if !sprint.HasBuild {
			add(sprint.Line, "sprint missing build scope", fmt.Sprintf("Sprint %d has no '#### Build' subsection", sprint.Number), "Add a '#### Build' subsection listing deliverables.")
		}
		if !sprint.HasGate {
			add(sprint.Line, "sprint missing acceptance gate", fmt.Sprintf("Sprint %d has no Acceptance, Release Gate, or Exit Gate subsection", sprint.Number), "Add an '#### Acceptance' subsection with checkable criteria.")
		}
	}
	for _, sprint := range sprints {
		for _, dependency := range sprint.DependsOn {
			if !numbers[dependency] {
				add(sprint.Line, "unknown dependency", fmt.Sprintf("Sprint %d depends on Sprint %d which is not defined", sprint.Number, dependency), "Reference a sprint number declared in this roadmap.")
			}
		}
	}
}
