package workspace

import (
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

var scaffoldFiles = map[string]string{
	"ultraplan.yml":              defaultConfig,
	"prompts/base.md":            "# Base Prompt\n\n",
	"prompts/synthesize.md":      "# Synthesis Prompt\n\n",
	"templates/repo-analysis.md": "# Repository Analysis\n\n",
	"templates/report.md":        "# Report\n\n",
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
	for _, dir := range []string{"prompts", "templates", "studies"} {
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
				if err := os.WriteFile(full, []byte(scaffoldFiles[op.Path]), 0o644); err != nil {
					return InitPlan{}, fmt.Errorf("create file %s: %w", op.Path, err)
				}
			}
		}
	}
	return plan, nil
}

func RequiredFiles() []string {
	return []string{"ultraplan.yml", "prompts/base.md", "prompts/synthesize.md", "templates/repo-analysis.md", "templates/report.md"}
}

func RequiredDirs() []string {
	return []string{"studies"}
}
