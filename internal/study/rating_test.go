package study

import "testing"

func TestParseRating(t *testing.T) {
	tests := []struct {
		name  string
		raw   string
		state RatingState
		score int
	}{
		{name: "fraction bold", raw: "**8 / 10**", state: RatingStateValid, score: 8},
		{name: "fraction plain", raw: "8/10", state: RatingStateValid, score: 8},
		{name: "label", raw: "Rating: 8", state: RatingStateValid, score: 8},
		{name: "missing", raw: "", state: RatingStateMissing},
		{name: "invalid", raw: "Rating: eleven", state: RatingStateInvalid},
		{name: "ambiguous", raw: "Rating: 8 and 7/10", state: RatingStateAmbiguous},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRating(tt.raw)
			if got.State != tt.state {
				t.Fatalf("State = %q, want %q", got.State, tt.state)
			}
			if tt.state == RatingStateValid && got.Score != tt.score {
				t.Fatalf("Score = %d, want %d", got.Score, tt.score)
			}
			if tt.state != RatingStateValid && got.Score != 0 {
				t.Fatalf("Score = %d, want 0", got.Score)
			}
		})
	}
}
