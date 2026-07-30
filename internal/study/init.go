package study

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

var (
	ErrInitValidation = errors.New("study init validation")
	ErrInitOverwrite  = errors.New("study init overwrite")
	ErrInitPartial    = errors.New("study init partial")
)

type InitRequest struct {
	WorkspaceRoot string
	InputPath     string
	OutputDir     string
	DryRun        bool
	Force         bool
	NoClone       bool
	CloneRunner   CloneRunner
}

type InitResult struct {
	StudyName     string
	StudyDir      string
	Directories   []string
	Files         []string
	CloneActions  []CloneAction
	Cloned        []CloneAction
	SkippedClones []CloneAction
	CloneFailures []CloneFailure
	DryRun        bool
	Forced        bool
}

type initPlan struct {
	def         initDefinition
	studyDir    string
	directories []string
	files       []plannedFile
	clones      []CloneAction
}

type plannedFile struct {
	path    string
	content []byte
}

func Init(req InitRequest) (InitResult, error) {
	plan, err := buildInitPlan(req)
	if err != nil {
		return InitResult{}, err
	}
	result := resultFromPlan(plan, req)
	if req.DryRun {
		return result, nil
	}
	if err := ensureWritable(plan, req.Force); err != nil {
		return result, err
	}
	for _, dir := range plan.directories {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return result, fmt.Errorf("create directory %s: %w", workspace.Rel(req.WorkspaceRoot, dir), err)
		}
	}
	for _, file := range plan.files {
		if err := os.MkdirAll(filepath.Dir(file.path), 0o755); err != nil {
			return result, fmt.Errorf("create parent for %s: %w", workspace.Rel(req.WorkspaceRoot, file.path), err)
		}
		if err := os.WriteFile(file.path, file.content, 0o644); err != nil {
			return result, fmt.Errorf("write %s: %w", workspace.Rel(req.WorkspaceRoot, file.path), err)
		}
	}
	if req.NoClone {
		result.SkippedClones = append(result.SkippedClones, plan.clones...)
		return result, nil
	}
	cloneResult := runCloneActions(req.CloneRunner, plan.clones)
	result.Cloned = cloneResult.Cloned
	result.CloneFailures = cloneResult.Failures
	if len(result.CloneFailures) > 0 {
		return result, ClonePartialError{Failures: result.CloneFailures}
	}
	return result, nil
}

func buildInitPlan(req InitRequest) (initPlan, error) {
	if req.WorkspaceRoot == "" {
		return initPlan{}, fmt.Errorf("%w: workspace root is required", ErrInitValidation)
	}
	def, err := parseInitYAML(req.InputPath)
	if err != nil {
		return initPlan{}, err
	}
	studyDir, err := resolveStudyOutput(req.WorkspaceRoot, def.Name, req.OutputDir)
	if err != nil {
		return initPlan{}, fmt.Errorf("%w: %v", ErrInitValidation, err)
	}
	dirs := []string{
		studyDir,
		filepath.Join(studyDir, "dimensions"),
		filepath.Join(studyDir, "sources"),
		filepath.Join(studyDir, "reports"),
		filepath.Join(studyDir, "reports", "source"),
		filepath.Join(studyDir, "reports", "final"),
	}
	var files []plannedFile
	files = append(files,
		plannedFile{path: filepath.Join(studyDir, "study-init.yml"), content: []byte(renderNormalizedYAML(def))},
		plannedFile{path: filepath.Join(studyDir, StudyConfigFileName), content: []byte(renderStudyConfigJSON())},
		plannedFile{path: filepath.Join(studyDir, "README.md"), content: []byte(renderReadme(def))},
	)
	for _, dim := range def.Dimensions {
		files = append(files, plannedFile{
			path:    filepath.Join(studyDir, "dimensions", dim.FileName),
			content: []byte(renderDimensionMarkdown(dim)),
		})
	}
	for _, source := range def.Sources {
		files = append(files, plannedFile{
			path:    filepath.Join(studyDir, "sources", source.Name+".ultraplan-source.yml"),
			content: []byte(renderSourceMetadataYAML(source)),
		})
	}
	var clones []CloneAction
	for _, source := range def.Sources {
		if source.URL == "" {
			continue
		}
		clones = append(clones, CloneAction{
			Name: source.Name,
			URL:  source.URL,
			Dest: filepath.Join(studyDir, "sources", source.Name),
		})
	}
	plan := initPlan{def: def, studyDir: studyDir, directories: dirs, files: files, clones: clones}
	if err := validatePlanPaths(req.WorkspaceRoot, plan); err != nil {
		return initPlan{}, err
	}
	return plan, nil
}

func resolveStudyOutput(root, studyName, outputDir string) (string, error) {
	if outputDir == "" {
		return workspace.ResolveInside(root, filepath.Join("studies", studyName))
	}
	return workspace.ResolveInside(root, outputDir)
}

func validatePlanPaths(root string, plan initPlan) error {
	for _, dir := range plan.directories {
		if !inside(plan.studyDir, dir) {
			return fmt.Errorf("%w: planned directory escapes study root: %s", ErrInitValidation, dir)
		}
		if _, err := workspace.ResolveInside(root, dir); err != nil {
			return fmt.Errorf("%w: %v", ErrInitValidation, err)
		}
	}
	for _, file := range plan.files {
		if !inside(plan.studyDir, file.path) {
			return fmt.Errorf("%w: planned file escapes study root: %s", ErrInitValidation, file.path)
		}
		if _, err := workspace.ResolveInside(root, file.path); err != nil {
			return fmt.Errorf("%w: %v", ErrInitValidation, err)
		}
	}
	for _, clone := range plan.clones {
		if !inside(plan.studyDir, clone.Dest) {
			return fmt.Errorf("%w: clone destination escapes study root: %s", ErrInitValidation, clone.Dest)
		}
	}
	return nil
}

func ensureWritable(plan initPlan, force bool) error {
	if _, err := os.Stat(plan.studyDir); err == nil && !force {
		return fmt.Errorf("%w: study directory already exists: %s; use --force to overwrite generated files", ErrInitOverwrite, plan.studyDir)
	} else if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect study directory %s: %w", plan.studyDir, err)
	}
	if !force {
		return nil
	}
	for _, file := range plan.files {
		if info, err := os.Stat(file.path); err == nil && info.IsDir() {
			return fmt.Errorf("%w: generated file path is a directory: %s", ErrInitOverwrite, file.path)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("inspect generated file %s: %w", file.path, err)
		}
	}
	return nil
}

func resultFromPlan(plan initPlan, req InitRequest) InitResult {
	result := InitResult{
		StudyName:    plan.def.Name,
		StudyDir:     plan.studyDir,
		DryRun:       req.DryRun,
		Forced:       req.Force,
		CloneActions: append([]CloneAction(nil), plan.clones...),
	}
	for _, dir := range plan.directories {
		result.Directories = append(result.Directories, dir)
	}
	for _, file := range plan.files {
		result.Files = append(result.Files, file.path)
	}
	return result
}

func inside(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}
