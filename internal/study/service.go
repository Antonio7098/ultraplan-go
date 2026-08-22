package study

import (
	"context"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

type Service struct {
	workspaceRoot string
	runtime       Runtime
	runtimeConfig runtimepkg.Request
}

type StudyListing struct {
	Study          Study
	Config         StudyConfig
	Sources        []Source
	Dimensions     []Dimension
	DimensionOrder []Dimension
}

type Option func(*Service)

type Runtime interface {
	StartRun(ctx context.Context, req runtimepkg.Request) (runtimepkg.Result, error)
}

type sessionDeleter interface {
	DeleteSession(context.Context, string) error
}

func (s Service) deleteCompletedSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	deleter, ok := s.runtime.(sessionDeleter)
	if !ok {
		return nil
	}
	return deleter.DeleteSession(ctx, sessionID)
}

func WithRuntime(rt Runtime, req runtimepkg.Request) Option {
	return func(s *Service) {
		s.runtime = rt
		s.runtimeConfig = req
	}
}

func NewService(workspaceRoot string, opts ...Option) Service {
	s := Service{workspaceRoot: workspaceRoot}
	for _, opt := range opts {
		opt(&s)
	}
	return s
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
	studyConfig, dimensionOrder, err := LoadStudyConfig(resolved, dimensions)
	if err != nil {
		return StudyListing{}, err
	}
	return StudyListing{
		Study:          resolved,
		Config:         studyConfig,
		Sources:        sources,
		Dimensions:     dimensions,
		DimensionOrder: dimensionOrder,
	}, nil
}

func (s Service) WriteSummary(studyRef string) (SummaryResult, error) {
	listing, err := s.ListStudy(studyRef)
	if err != nil {
		return SummaryResult{}, err
	}
	return WriteSummary(listing.Study, listing.Dimensions, listing.Sources)
}
