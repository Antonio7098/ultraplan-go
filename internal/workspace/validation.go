package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

type ValidationResult struct {
	Valid  bool     `json:"valid"`
	Issues []string `json:"issues,omitempty"`
}

func Validate(root string) ValidationResult {
	var issues []string
	for _, rel := range RequiredFiles() {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || info.IsDir() {
			issues = append(issues, fmt.Sprintf("missing required file: %s", rel))
		}
	}
	for _, rel := range RequiredDirs() {
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || !info.IsDir() {
			issues = append(issues, fmt.Sprintf("missing required directory: %s", rel))
		}
	}
	return ValidationResult{Valid: len(issues) == 0, Issues: issues}
}
