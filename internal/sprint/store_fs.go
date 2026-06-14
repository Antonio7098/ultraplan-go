package sprint

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ultraplan-go/internal/project"
	"ultraplan-go/internal/workspace"
)

type FSStore struct {
	Root string
}

type ArtifactSnapshot struct {
	Files               map[PlanningStage]bool
	AreaReasoningFiles  []string
	NoReasoningSelected bool
}

type PlanningInputs struct {
	Requirements string
	SprintIndex  string
	ProjectIndex string
	Docs         []string
}

func NewFSStore(root string) FSStore {
	return FSStore{Root: root}
}

func (s FSStore) ReadArtifacts(sp Sprint) (ArtifactSnapshot, error) {
	snap := ArtifactSnapshot{Files: map[PlanningStage]bool{}}
	for _, stage := range []PlanningStage{StageRequirements, StageSprintIndex, StageTechnicalHandbook, StageReasoning, StagePlan} {
		path, err := ArtifactPath(s.Root, sp, stage)
		if err != nil {
			return ArtifactSnapshot{}, err
		}
		ok, err := nonEmptyFile(path)
		if err != nil {
			return ArtifactSnapshot{}, err
		}
		snap.Files[stage] = ok
	}
	reasoningDir, err := ArtifactPath(s.Root, sp, StageAreaReasoning)
	if err != nil {
		return ArtifactSnapshot{}, err
	}
	entries, err := readOptionalDir(reasoningDir)
	if err != nil {
		return ArtifactSnapshot{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || isHidden(entry.Name()) || strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
			continue
		}
		full := filepath.Join(reasoningDir, entry.Name())
		ok, err := nonEmptyFile(full)
		if err != nil {
			return ArtifactSnapshot{}, err
		}
		if ok {
			snap.AreaReasoningFiles = append(snap.AreaReasoningFiles, entry.Name())
		}
	}
	sort.Strings(snap.AreaReasoningFiles)
	indexPath, err := ArtifactPath(s.Root, sp, StageSprintIndex)
	if err != nil {
		return ArtifactSnapshot{}, err
	}
	content, err := os.ReadFile(indexPath)
	if err == nil {
		snap.NoReasoningSelected = explicitlyNoReasoningTemplates(string(content))
	}
	return snap, nil
}

func (s FSStore) ReadPlanningInputs(sp Sprint) (PlanningInputs, error) {
	var inputs PlanningInputs
	req, err := ArtifactPath(s.Root, sp, StageRequirements)
	if err != nil {
		return PlanningInputs{}, err
	}
	if data, err := os.ReadFile(req); err == nil {
		inputs.Requirements = string(data)
	} else {
		return PlanningInputs{}, err
	}
	idx, err := ArtifactPath(s.Root, sp, StageSprintIndex)
	if err != nil {
		return PlanningInputs{}, err
	}
	if data, err := os.ReadFile(idx); err == nil {
		inputs.SprintIndex = string(data)
	} else if !os.IsNotExist(err) {
		return PlanningInputs{}, err
	}
	projectRoot, err := workspace.ResolveInside(s.Root, filepath.ToSlash(filepath.Join("projects", sp.Project)))
	if err != nil {
		return PlanningInputs{}, err
	}
	data, err := os.ReadFile(filepath.Join(projectRoot, "project-index.md"))
	if err != nil {
		return PlanningInputs{}, err
	}
	inputs.ProjectIndex = string(data)
	p := project.Project{Name: sp.Project, Path: projectRoot}
	files, err := project.NewFSStore(s.Root).ReadProjectFiles(p)
	if err != nil {
		return PlanningInputs{}, err
	}
	inputs.Docs = files.MarkdownDocs
	return inputs, nil
}

func nonEmptyFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !info.IsDir() && info.Size() > 0, nil
}

func explicitlyNoReasoningTemplates(content string) bool {
	lower := strings.ToLower(content)
	needles := []string{
		"no reasoning templates",
		"none selected",
		"no templates selected",
		"selected reasoning templates\n\nnone",
	}
	for _, needle := range needles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
