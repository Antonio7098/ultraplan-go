package project

import "sort"

type Service struct {
	root             string
	store            FSStore
	reasoningRuntime ReasoningRuntime
}

func NewService(root string) Service {
	return Service{root: root, store: NewFSStore(root)}
}

func (s Service) ListProjects() ([]Project, error) {
	return DiscoverProjects(s.root)
}

func (s Service) Status(ref string) (ProjectStatus, error) {
	p, files, err := s.resolveAndRead(ref)
	if err != nil {
		return ProjectStatus{}, err
	}
	validation := ValidateProject(s.root, p, files)
	status := StatusFromValidation(p, files, validation)
	for _, rel := range ReasoningDefaultPaths() {
		resolved, resolveErr := ResolveReasoningDefault(s.root, p.Name, rel)
		if resolveErr != nil {
			status.ReasoningDefaults = append(status.ReasoningDefaults, ReasoningDefault{
				RelativePath: rel,
				Source:       "invalid",
			})
			continue
		}
		resolved.Content = ""
		status.ReasoningDefaults = append(status.ReasoningDefaults, resolved)
	}
	index, _ := ParseProjectIndex(files.IndexContent)
	projectReasoningPrefix := "projects/" + p.Name + "/reasoning/"
	for _, entry := range index.Entries {
		path := normalizeCatalogPath(entry.Path)
		if entry.Section == SectionAvailableReasoningTemplate && len(path) > len(projectReasoningPrefix) && path[:len(projectReasoningPrefix)] == projectReasoningPrefix {
			status.SprintReasoningTemplates = append(status.SprintReasoningTemplates, path)
		}
	}
	sort.Strings(status.SprintReasoningTemplates)
	status.AreaReasoningDocuments = append([]string(nil), status.SprintReasoningTemplates...)
	status.ProjectReasoning, _ = s.ReasoningStatus(ref)
	return status, nil
}

func (s Service) Validate(ref string) (ValidationResult, error) {
	p, files, err := s.resolveAndRead(ref)
	if err != nil {
		return ValidationResult{}, err
	}
	return ValidateProject(s.root, p, files), nil
}

func (s Service) resolveAndRead(ref string) (Project, ProjectFiles, error) {
	projects, err := DiscoverProjects(s.root)
	if err != nil {
		return Project{}, ProjectFiles{}, err
	}
	p, err := ResolveProject(projects, ref)
	if err != nil {
		return Project{}, ProjectFiles{}, err
	}
	files, err := s.store.ReadProjectFiles(p)
	if err != nil {
		return Project{}, ProjectFiles{}, err
	}
	return p, files, nil
}
