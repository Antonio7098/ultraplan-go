package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

const (
	AreaReasoningPromptPath    = "prompts/create-area-reasoning.md"
	FinalReasoningPromptPath   = "prompts/create-sprint-reasoning.md"
	FinalReasoningTemplatePath = "templates/sprint-reasoning.md"
)

var reasoningDefaultPaths = []string{
	AreaReasoningPromptPath,
	FinalReasoningPromptPath,
	FinalReasoningTemplatePath,
}

type ReasoningDefault struct {
	RelativePath string
	Source       string
	Path         string
	Content      string
}

func ReasoningDefaultPaths() []string {
	return append([]string(nil), reasoningDefaultPaths...)
}

func IsReasoningDefault(path string) bool {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	for _, candidate := range reasoningDefaultPaths {
		if path == candidate {
			return true
		}
	}
	return false
}

func normalizeCatalogPath(path string) string {
	path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, ".ultra/")
	return path
}

func ResolveReasoningDefault(root, projectName, rel string) (ReasoningDefault, error) {
	rel = filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if !IsSafeName(projectName) {
		return ReasoningDefault{}, fmt.Errorf("invalid project name %q", projectName)
	}
	if !IsReasoningDefault(rel) {
		return ReasoningDefault{}, fmt.Errorf("%q is not a supported project reasoning override", rel)
	}

	projectRel := filepath.ToSlash(filepath.Join("projects", projectName, filepath.FromSlash(rel)))
	if resolved, found, err := readReasoningDefault(root, projectRel, "project:"+projectRel); err != nil {
		return ReasoningDefault{}, err
	} else if found {
		resolved.RelativePath = rel
		return resolved, nil
	}

	if resolved, found, err := readReasoningDefault(root, rel, "workspace:"+rel); err != nil {
		return ReasoningDefault{}, err
	} else if found {
		resolved.RelativePath = rel
		return resolved, nil
	}

	content, ok := workspace.DefaultOverrideFile(rel)
	if !ok {
		return ReasoningDefault{}, fmt.Errorf("no project, workspace, or built-in default exists for %q", rel)
	}
	return ReasoningDefault{
		RelativePath: rel,
		Source:       "builtin:" + rel,
		Path:         "builtin:" + rel,
		Content:      content,
	}, nil
}

func readReasoningDefault(root, rel, source string) (ReasoningDefault, bool, error) {
	full, err := workspace.ResolveInside(root, rel)
	if err != nil {
		return ReasoningDefault{}, false, fmt.Errorf("resolve reasoning default %q: %w", rel, err)
	}
	info, err := os.Stat(full)
	if os.IsNotExist(err) {
		return ReasoningDefault{}, false, nil
	}
	if err != nil {
		return ReasoningDefault{}, false, fmt.Errorf("inspect reasoning default %q: %w", rel, err)
	}
	if info.IsDir() {
		return ReasoningDefault{}, false, fmt.Errorf("reasoning default %q is a directory", rel)
	}
	if strings.ToLower(filepath.Ext(full)) != ".md" {
		return ReasoningDefault{}, false, fmt.Errorf("reasoning default %q is not Markdown", rel)
	}
	content, err := os.ReadFile(full)
	if err != nil {
		return ReasoningDefault{}, false, fmt.Errorf("read reasoning default %q: %w", rel, err)
	}
	if strings.TrimSpace(string(content)) == "" {
		return ReasoningDefault{}, false, fmt.Errorf("reasoning default %q is empty", rel)
	}
	return ReasoningDefault{Source: source, Path: rel, Content: string(content)}, true, nil
}
