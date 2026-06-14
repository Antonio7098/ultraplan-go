package project

type Service struct {
	root  string
	store FSStore
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
	return StatusFromValidation(p, files, validation), nil
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
