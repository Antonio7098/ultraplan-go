package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type DefaultsPlan struct {
	Root       string      `json:"root"`
	Operations []Operation `json:"operations"`
}

type DefaultsOptions struct {
	Force bool
}

func PlanDefaults(path string, opts DefaultsOptions) (DefaultsPlan, error) {
	root, err := normalize(path)
	if err != nil {
		return DefaultsPlan{}, err
	}
	plan := DefaultsPlan{Root: root}
	dirs := []string{"prompts", "templates"}
	for _, dir := range dirs {
		full, err := ResolveInside(root, dir)
		if err != nil {
			return DefaultsPlan{}, err
		}
		if _, statErr := os.Stat(full); os.IsNotExist(statErr) {
			plan.Operations = append(plan.Operations, Operation{Action: "create", Path: dir, Type: "dir"})
		}
	}
	files := DefaultOverrideFiles()
	paths := make([]string, 0, len(files))
	for rel := range files {
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		full, err := ResolveInside(root, rel)
		if err != nil {
			return DefaultsPlan{}, err
		}
		current, err := os.ReadFile(full)
		switch {
		case os.IsNotExist(err):
			plan.Operations = append(plan.Operations, Operation{Action: "create", Path: rel, Type: "file"})
		case err != nil:
			return DefaultsPlan{}, fmt.Errorf("read existing default override %s: %w", rel, err)
		case string(current) == files[rel]:
			continue
		case opts.Force:
			plan.Operations = append(plan.Operations, Operation{Action: "overwrite", Path: rel, Type: "file"})
		default:
			plan.Operations = append(plan.Operations, Operation{Action: "skip", Path: rel, Type: "file"})
		}
	}
	return plan, nil
}

func InstallDefaults(path string, opts DefaultsOptions) (DefaultsPlan, error) {
	plan, err := PlanDefaults(path, opts)
	if err != nil {
		return DefaultsPlan{}, err
	}
	files := DefaultOverrideFiles()
	for _, op := range plan.Operations {
		if op.Action == "skip" {
			continue
		}
		full, err := ResolveInside(plan.Root, filepath.FromSlash(op.Path))
		if err != nil {
			return DefaultsPlan{}, err
		}
		switch op.Type {
		case "dir":
			if err := os.MkdirAll(full, 0o755); err != nil {
				return DefaultsPlan{}, fmt.Errorf("create directory %s: %w", op.Path, err)
			}
		case "file":
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				return DefaultsPlan{}, fmt.Errorf("create parent for %s: %w", op.Path, err)
			}
			if err := os.WriteFile(full, []byte(files[op.Path]), 0o644); err != nil {
				return DefaultsPlan{}, fmt.Errorf("%s file %s: %w", op.Action, op.Path, err)
			}
		}
	}
	return plan, nil
}
