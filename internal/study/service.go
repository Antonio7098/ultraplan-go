package study

type Service struct {
	workspaceRoot string
}

type StudyListing struct {
	Study      Study
	Sources    []Source
	Dimensions []Dimension
}

func NewService(workspaceRoot string) Service {
	return Service{workspaceRoot: workspaceRoot}
}

func (s Service) ListStudies() ([]Study, error) {
	return DiscoverStudies(s.workspaceRoot)
}

func (s Service) ListStudy(ref string) (StudyListing, error) {
	studies, err := DiscoverStudies(s.workspaceRoot)
	if err != nil {
		return StudyListing{}, err
	}
	resolved, err := ResolveStudy(studies, ref)
	if err != nil {
		return StudyListing{}, err
	}
	sources, err := DiscoverSources(resolved)
	if err != nil {
		return StudyListing{}, err
	}
	dimensions, err := DiscoverDimensions(resolved)
	if err != nil {
		return StudyListing{}, err
	}
	return StudyListing{
		Study:      resolved,
		Sources:    sources,
		Dimensions: dimensions,
	}, nil
}
