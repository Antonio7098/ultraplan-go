package app

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/Antonio7098/ultraplan-go/internal/study"
	"github.com/Antonio7098/ultraplan-go/internal/workspace"
)

// WebRepoDimensionScore is one repo's rating for a single dimension, linked to
// its source report.
type WebRepoDimensionScore struct {
	Dimension   string
	Number      string
	Slug        string
	Score       int
	Ref         string
	DisplayPath string
}

// WebRepoScore aggregates one repo's ratings across every applicable dimension.
type WebRepoScore struct {
	Name              string
	Average           float64
	Total             int
	RatedCount        int
	Applicable        int
	ApplicableNumbers []string
	Best              *WebRepoDimensionScore
	Scores            []WebRepoDimensionScore
}

// WebDimensionTopRepo names the leading repos for one dimension.
type WebDimensionTopRepo struct {
	Dimension string
	Number    string
	Slug      string
	Leaders   []WebRepoLeader
}

// WebRepoLeader is one ranked repo inside a dimension's leader list.
type WebRepoLeader struct {
	Rank  int
	Name  string
	Score int
	Ref   string
}

// WebStudyReposResult is the bounded repo score board shown on study pages.
type WebStudyReposResult struct {
	Repos      []WebRepoScore
	Dimensions []WebDimensionTopRepo
	CollectionInfo
}

// StudyRepos ranks the study's repos by their source-report ratings: each repo
// gets an average and total across applicable dimensions plus its best
// dimension, and each dimension names its highest scoring repos.
func (u *webUseCases) StudyRepos(ctx context.Context, name string) (WebStudyReposResult, error) {
	if err := ctx.Err(); err != nil {
		return WebStudyReposResult{}, err
	}
	listing, err := study.NewService(u.root).ListStudy(name)
	if err != nil {
		return WebStudyReposResult{}, fmt.Errorf("%w: study repos", ErrWebNotFound)
	}
	repos := make([]WebRepoScore, 0, len(listing.Sources))
	dimensionTop := make(map[string][]WebRepoLeader, len(listing.Dimensions))
	for _, source := range listing.Sources {
		if err := ctx.Err(); err != nil {
			return WebStudyReposResult{}, err
		}
		repo := WebRepoScore{Name: source.Name}
		for _, dimension := range listing.Dimensions {
			if !study.SourceAppliesToDimension(source, dimension) {
				continue
			}
			repo.Applicable++
			repo.ApplicableNumbers = append(repo.ApplicableNumbers, dimension.Number)
			path := study.SourceReportPath(listing.Study, source, dimension)
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			rating := study.ReportRating(string(content))
			if rating.State != study.RatingStateValid {
				continue
			}
			ref, displayPath := u.reportLink(path)
			if ref == "" {
				continue
			}
			repo.RatedCount++
			repo.Total += rating.Score
			entry := WebRepoDimensionScore{Dimension: dimension.Ref(), Number: dimension.Number, Slug: dimension.Slug, Score: rating.Score, Ref: ref, DisplayPath: displayPath}
			repo.Scores = append(repo.Scores, entry)
			leader := WebRepoLeader{Name: source.Name, Score: rating.Score, Ref: ref}
			dimensionTop[dimension.Number] = append(dimensionTop[dimension.Number], leader)
		}
		if repo.Applicable == 0 {
			continue
		}
		if repo.RatedCount > 0 {
			repo.Average = math.Round(float64(repo.Total)/float64(repo.RatedCount)*10) / 10
			best := repo.Scores[0]
			for _, entry := range repo.Scores[1:] {
				if entry.Score > best.Score || (entry.Score == best.Score && entry.Dimension < best.Dimension) {
					best = entry
				}
			}
			repo.Best = &best
		}
		sort.Slice(repo.Scores, func(i, j int) bool { return repo.Scores[i].Number < repo.Scores[j].Number })
		repos = append(repos, repo)
	}
	sort.SliceStable(repos, func(i, j int) bool {
		left, right := repos[i], repos[j]
		if left.Average != right.Average {
			return left.Average > right.Average
		}
		if left.RatedCount != right.RatedCount {
			return left.RatedCount > right.RatedCount
		}
		return left.Name < right.Name
	})
	total := len(repos)
	if total > WebCollectionLimit {
		repos = repos[:WebCollectionLimit]
	}
	topByRef := make(map[string]string, len(repos))
	for _, repo := range repos {
		for _, entry := range repo.Scores {
			topByRef[repo.Name+"|"+entry.Number] = entry.Ref
		}
	}
	champions := make([]WebDimensionTopRepo, 0, len(listing.Dimensions))
	for _, dimension := range listing.Dimensions {
		leaders := dimensionTop[dimension.Number]
		if len(leaders) == 0 {
			continue
		}
		sort.SliceStable(leaders, func(i, j int) bool {
			if leaders[i].Score != leaders[j].Score {
				return leaders[i].Score > leaders[j].Score
			}
			return leaders[i].Name < leaders[j].Name
		})
		cut := len(leaders)
		if cut > 3 {
			cut = 3
		}
		ranked := make([]WebRepoLeader, 0, cut)
		for index, leader := range leaders[:cut] {
			leader.Rank = index + 1
			leader.Ref = topByRef[leader.Name+"|"+dimension.Number]
			ranked = append(ranked, leader)
		}
		champions = append(champions, WebDimensionTopRepo{Dimension: dimension.Ref(), Number: dimension.Number, Slug: dimension.Slug, Leaders: ranked})
	}
	return WebStudyReposResult{
		Repos:          repos,
		Dimensions:     champions,
		CollectionInfo: collectionInfo(len(repos), total),
	}, nil
}

func (u *webUseCases) reportLink(path string) (string, string) {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", ""
	}
	rel := workspace.Rel(u.root, path)
	if !supportedPreviewPath(rel) {
		return "", ""
	}
	return u.issue("artifact", rel), rel
}
