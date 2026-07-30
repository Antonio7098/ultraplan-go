package workspace

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

const SkillsRoot = ".opencode/skills"

//go:embed scaffold/skills/requirements/SKILL.md
var requirementsSkill string

//go:embed scaffold/skills/sprint-index/SKILL.md
var sprintIndexSkill string

//go:embed scaffold/skills/technical-handbook/SKILL.md
var technicalHandbookSkill string

//go:embed scaffold/skills/area-reasoning/SKILL.md
var areaReasoningSkill string

//go:embed scaffold/skills/reasoning/SKILL.md
var reasoningSkill string

//go:embed scaffold/skills/plan/SKILL.md
var planSkill string

//go:embed scaffold/skills/execute/SKILL.md
var executeSkill string

//go:embed scaffold/skills/review/SKILL.md
var reviewSkill string

//go:embed scaffold/skills/smoke/SKILL.md
var smokeSkill string

var embeddedSkills = map[string]string{
	"requirements":        requirementsSkill,
	"sprint-index":       sprintIndexSkill,
	"technical-handbook": technicalHandbookSkill,
	"area-reasoning":     areaReasoningSkill,
	"reasoning":          reasoningSkill,
	"plan":               planSkill,
	"execute":            executeSkill,
	"review":             reviewSkill,
	"smoke":              smokeSkill,
}

type SkillMaterializeOptions struct {
	Force bool
}

type SkillMaterializePlan struct {
	Root       string      `json:"root"`
	Operations []Operation `json:"operations"`
}

func SkillNames() []string {
	names := make([]string, 0, len(embeddedSkills))
	for name := range embeddedSkills {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func EmbeddedSkill(name string) (string, bool) {
	content, ok := embeddedSkills[name]
	return content, ok
}

func PlanSkillMaterialize(path, name string, opts SkillMaterializeOptions) (SkillMaterializePlan, error) {
	root, err := normalize(path)
	if err != nil {
		return SkillMaterializePlan{}, err
	}
	selected, err := selectedSkills(name)
	if err != nil {
		return SkillMaterializePlan{}, err
	}
	plan := SkillMaterializePlan{Root: root}
	for _, skillName := range selected {
		rel := filepath.ToSlash(filepath.Join(SkillsRoot, skillName, "SKILL.md"))
		full, err := ResolveInside(root, filepath.FromSlash(rel))
		if err != nil {
			return SkillMaterializePlan{}, err
		}
		current, readErr := os.ReadFile(full)
		switch {
		case os.IsNotExist(readErr):
			plan.Operations = append(plan.Operations, Operation{Action: "create", Path: rel, Type: "file"})
		case readErr != nil:
			return SkillMaterializePlan{}, fmt.Errorf("read existing skill %s: %w", rel, readErr)
		case string(current) == embeddedSkills[skillName]:
			continue
		case opts.Force:
			plan.Operations = append(plan.Operations, Operation{Action: "overwrite", Path: rel, Type: "file"})
		default:
			plan.Operations = append(plan.Operations, Operation{Action: "skip", Path: rel, Type: "file"})
		}
	}
	return plan, nil
}

func MaterializeSkills(path, name string, opts SkillMaterializeOptions) (SkillMaterializePlan, error) {
	plan, err := PlanSkillMaterialize(path, name, opts)
	if err != nil {
		return SkillMaterializePlan{}, err
	}
	for _, op := range plan.Operations {
		if op.Action == "skip" {
			continue
		}
		full, err := ResolveInside(plan.Root, filepath.FromSlash(op.Path))
		if err != nil {
			return SkillMaterializePlan{}, err
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return SkillMaterializePlan{}, fmt.Errorf("create skill directory for %s: %w", op.Path, err)
		}
		skillName := filepath.Base(filepath.Dir(op.Path))
		if err := os.WriteFile(full, []byte(embeddedSkills[skillName]), 0o644); err != nil {
			return SkillMaterializePlan{}, fmt.Errorf("%s skill %s: %w", op.Action, op.Path, err)
		}
	}
	return plan, nil
}

func selectedSkills(name string) ([]string, error) {
	if name == "all" {
		return SkillNames(), nil
	}
	if _, ok := embeddedSkills[name]; !ok {
		return nil, fmt.Errorf("unknown skill %q", name)
	}
	return []string{name}, nil
}
