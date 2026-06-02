package study

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	ratingFractionPattern = regexp.MustCompile(`(?i)\b(\d{1,2})\s*/\s*10\b`)
	ratingLabelPattern    = regexp.MustCompile(`(?i)\brating:\s*(\d{1,2})\b`)
)

func ParseRating(raw string) RatingResult {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return RatingResult{State: RatingStateMissing}
	}
	fractions := ratingFractionPattern.FindAllStringSubmatch(trimmed, -1)
	labels := ratingLabelPattern.FindAllStringSubmatch(trimmed, -1)

	scores := make(map[int]struct{})
	for _, match := range fractions {
		score, ok := parseScore(match[1])
		if !ok {
			return RatingResult{State: RatingStateInvalid, Reason: "rating score must be between 0 and 10"}
		}
		scores[score] = struct{}{}
	}
	for _, match := range labels {
		score, ok := parseScore(match[1])
		if !ok {
			return RatingResult{State: RatingStateInvalid, Reason: "rating score must be between 0 and 10"}
		}
		scores[score] = struct{}{}
	}
	if len(scores) > 1 {
		return RatingResult{State: RatingStateAmbiguous, Reason: "multiple rating values found"}
	}
	if len(scores) == 1 {
		for score := range scores {
			return RatingResult{State: RatingStateValid, Score: score, Raw: raw}
		}
	}
	if strings.Count(strings.ToLower(trimmed), "rating") > 1 {
		return RatingResult{State: RatingStateAmbiguous, Reason: "multiple rating labels found"}
	}
	return RatingResult{State: RatingStateInvalid, Reason: "unsupported rating format"}
}

func parseScore(raw string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	if n < 0 || n > 10 {
		return 0, false
	}
	return n, true
}
