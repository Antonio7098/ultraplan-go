package study

func GetApplicableSources(sources []Source, dimension Dimension) []Source {
	out := make([]Source, 0, len(sources))
	for _, source := range sources {
		if source.Kind == SourceKindDirectory || len(source.ApplicableDimensions) == 0 {
			out = append(out, source)
			continue
		}
		for _, applicable := range source.ApplicableDimensions {
			if applicable == dimension.Number {
				out = append(out, source)
				break
			}
		}
	}
	return out
}
