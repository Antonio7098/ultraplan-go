package workspace

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

type Operation struct {
	Action string `json:"action"`
	Path   string `json:"path"`
	Type   string `json:"type"`
}

type InitPlan struct {
	Root       string      `json:"root"`
	Operations []Operation `json:"operations"`
}

//go:embed scaffold/prompts/base.md
var defaultBasePrompt string

//go:embed scaffold/prompts/synthesize.md
var defaultSynthesizePrompt string

//go:embed scaffold/prompts/create-area-reasoning.md
var defaultCreateAreaReasoningPrompt string

//go:embed scaffold/prompts/create-requirements.md
var defaultCreateRequirementsPrompt string

//go:embed scaffold/prompts/create-sprint-index.md
var defaultCreateSprintIndexPrompt string

//go:embed scaffold/prompts/create-sprint-reasoning.md
var defaultCreateSprintReasoningPrompt string

//go:embed scaffold/prompts/create-technical-handbook.md
var defaultCreateTechnicalHandbookPrompt string

//go:embed scaffold/prompts/execute-sprint.md
var defaultExecuteSprintPrompt string

//go:embed scaffold/prompts/meta-plan.md
var defaultMetaPlanPrompt string

//go:embed scaffold/prompts/meta-synthesize.md
var defaultMetaSynthesizePrompt string

//go:embed scaffold/prompts/plan-sprint.md
var defaultPlanSprintPrompt string

//go:embed scaffold/prompts/review.md
var defaultReviewPrompt string

//go:embed scaffold/templates/README.md
var defaultTemplatesReadme string

//go:embed scaffold/templates/meta-report.md
var defaultMetaReportTemplate string

//go:embed scaffold/templates/project-index.md
var defaultProjectIndexTemplate string

//go:embed scaffold/templates/repo-analysis.md
var defaultRepoAnalysisTemplate string

//go:embed scaffold/templates/report.md
var defaultReportTemplate string

//go:embed scaffold/templates/requirements.md
var defaultRequirementsTemplate string

//go:embed scaffold/templates/review.md
var defaultReviewTemplate string

//go:embed scaffold/templates/sprint-index.md
var defaultSprintIndexTemplate string

//go:embed scaffold/templates/sprint-plan.md
var defaultSprintPlanTemplate string

//go:embed scaffold/templates/sprint-reasoning.md
var defaultSprintReasoningTemplate string

//go:embed scaffold/templates/technical-handbook.md
var defaultTechnicalHandbookTemplate string

var workspaceFiles = map[string]string{
	"ultraplan.yml": defaultConfig,
}

var defaultOverrideFiles = map[string]string{
	"prompts/base.md":                      defaultBasePrompt,
	"prompts/create-area-reasoning.md":     defaultCreateAreaReasoningPrompt,
	"prompts/create-requirements.md":       defaultCreateRequirementsPrompt,
	"prompts/create-sprint-index.md":       defaultCreateSprintIndexPrompt,
	"prompts/create-sprint-reasoning.md":   defaultCreateSprintReasoningPrompt,
	"prompts/create-technical-handbook.md": defaultCreateTechnicalHandbookPrompt,
	"prompts/execute-sprint.md":            defaultExecuteSprintPrompt,
	"prompts/meta-plan.md":                 defaultMetaPlanPrompt,
	"prompts/meta-synthesize.md":           defaultMetaSynthesizePrompt,
	"prompts/plan-sprint.md":               defaultPlanSprintPrompt,
	"prompts/review.md":                    defaultReviewPrompt,
	"prompts/synthesize.md":                defaultSynthesizePrompt,
	"templates/README.md":                  defaultTemplatesReadme,
	"templates/meta-report.md":             defaultMetaReportTemplate,
	"templates/project-index.md":           defaultProjectIndexTemplate,
	"templates/repo-analysis.md":           defaultRepoAnalysisTemplate,
	"templates/report.md":                  defaultReportTemplate,
	"templates/requirements.md":            defaultRequirementsTemplate,
	"templates/review.md":                  defaultReviewTemplate,
	"templates/sprint-index.md":            defaultSprintIndexTemplate,
	"templates/sprint-plan.md":             defaultSprintPlanTemplate,
	"templates/sprint-reasoning.md":        defaultSprintReasoningTemplate,
	"templates/technical-handbook.md":      defaultTechnicalHandbookTemplate,
}

const defaultConfig = `version: 1
runtime:
  default: opencode
models:
  default: provider/model
  primary: provider/model
  backup: provider/model
execution:
  default_variant: high
  default_parallel: 3
  default_timeout: 30m
  default_retries: 3
logging:
  format: text
  level: info
agentwrap:
  executable: opencode
  required_health:
    - runtime_available
    - structured_output
    - workdir
`

func PlanInit(path string) (InitPlan, error) {
	root, err := normalize(path)
	if err != nil {
		return InitPlan{}, err
	}
	plan := InitPlan{Root: root}
	for _, dir := range RequiredDirs() {
		full, err := ResolveInside(root, dir)
		if err != nil {
			return InitPlan{}, err
		}
		if _, statErr := os.Stat(full); os.IsNotExist(statErr) {
			plan.Operations = append(plan.Operations, Operation{Action: "create", Path: dir, Type: "dir"})
		}
	}
	for _, rel := range RequiredFiles() {
		full, err := ResolveInside(root, rel)
		if err != nil {
			return InitPlan{}, err
		}
		if _, statErr := os.Stat(full); os.IsNotExist(statErr) {
			plan.Operations = append(plan.Operations, Operation{Action: "create", Path: rel, Type: "file"})
		}
	}
	return plan, nil
}

func Init(path string) (InitPlan, error) {
	plan, err := PlanInit(path)
	if err != nil {
		return InitPlan{}, err
	}
	for _, op := range plan.Operations {
		full, err := ResolveInside(plan.Root, filepath.FromSlash(op.Path))
		if err != nil {
			return InitPlan{}, err
		}
		switch op.Type {
		case "dir":
			if err := os.MkdirAll(full, 0o755); err != nil {
				return InitPlan{}, fmt.Errorf("create directory %s: %w", op.Path, err)
			}
		case "file":
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return InitPlan{}, fmt.Errorf("create parent for %s: %w", op.Path, err)
			}
			if _, err := os.Stat(full); os.IsNotExist(err) {
				if err := os.WriteFile(full, []byte(workspaceFiles[op.Path]), 0o644); err != nil {
					return InitPlan{}, fmt.Errorf("create file %s: %w", op.Path, err)
				}
			}
		}
	}
	return plan, nil
}

func RequiredFiles() []string {
	return []string{"ultraplan.yml"}
}

func RequiredDirs() []string {
	return []string{"studies"}
}

func DefaultOverrideFiles() map[string]string {
	out := make(map[string]string, len(defaultOverrideFiles))
	for rel, content := range defaultOverrideFiles {
		out[rel] = content
	}
	return out
}

func DefaultOverrideFile(rel string) (string, bool) {
	content, ok := defaultOverrideFiles[filepath.ToSlash(rel)]
	return content, ok
}
