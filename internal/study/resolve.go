package study

import (
	"fmt"
	"strings"
)

type RefKind string

const (
	RefKindStudy     RefKind = "study"
	RefKindSource    RefKind = "source"
	RefKindDimension RefKind = "dimension"
)

type RefError struct {
	Kind       RefKind
	Ref        string
	Candidates []string
	Ambiguous  bool
}

func (e RefError) Error() string {
	if e.Ambiguous {
		return fmt.Sprintf("ambiguous %s reference %q; matches: %s", e.Kind, e.Ref, strings.Join(e.Candidates, ", "))
	}
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("%s reference %q not found", e.Kind, e.Ref)
	}
	return fmt.Sprintf("%s reference %q not found; available: %s", e.Kind, e.Ref, strings.Join(e.Candidates, ", "))
}

func ResolveStudy(studies []Study, ref string) (Study, error) {
	index := make([]matchCandidate[Study], 0, len(studies))
	for _, s := range studies {
		index = append(index, matchCandidate[Study]{value: s, canonical: s.Name, aliases: []string{s.Name}})
	}
	return resolve(index, RefKindStudy, ref)
}

func ResolveSource(sources []Source, ref string) (Source, error) {
	index := make([]matchCandidate[Source], 0, len(sources))
	for _, s := range sources {
		index = append(index, matchCandidate[Source]{value: s, canonical: s.Name, aliases: []string{s.Name}})
	}
	return resolve(index, RefKindSource, ref)
}

func ResolveDimension(dimensions []Dimension, ref string) (Dimension, error) {
	normalizedRef := normalizeDimensionRef(ref)
	index := make([]matchCandidate[Dimension], 0, len(dimensions))
	for _, d := range dimensions {
		aliases := []string{d.Number, d.Slug, d.File, d.Ref()}
		index = append(index, matchCandidate[Dimension]{value: d, canonical: d.Ref(), aliases: compactAliases(aliases)})
	}
	return resolve(index, RefKindDimension, normalizedRef)
}

type matchCandidate[T any] struct {
	value     T
	canonical string
	aliases   []string
}

func resolve[T any](candidates []matchCandidate[T], kind RefKind, ref string) (T, error) {
	var zero T
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return zero, RefError{Kind: kind, Ref: ref, Candidates: canonicalNames(candidates)}
	}
	for _, candidate := range candidates {
		for _, alias := range candidate.aliases {
			if alias == ref {
				return candidate.value, nil
			}
		}
	}
	var matches []matchCandidate[T]
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		for _, alias := range candidate.aliases {
			if strings.HasPrefix(alias, ref) {
				if !seen[candidate.canonical] {
					matches = append(matches, candidate)
					seen[candidate.canonical] = true
				}
				break
			}
		}
	}
	switch len(matches) {
	case 0:
		return zero, RefError{Kind: kind, Ref: ref, Candidates: canonicalNames(candidates)}
	case 1:
		return matches[0].value, nil
	default:
		return zero, RefError{Kind: kind, Ref: ref, Candidates: canonicalNames(matches), Ambiguous: true}
	}
}

func canonicalNames[T any](candidates []matchCandidate[T]) []string {
	names := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		names = append(names, candidate.canonical)
	}
	return names
}

func compactAliases(aliases []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(aliases))
	for _, alias := range aliases {
		alias = strings.TrimSpace(alias)
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		out = append(out, alias)
	}
	return out
}
