package study

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReadRunHistoryIgnoresOnlyMalformedTrailingRecord(t *testing.T) {
	valid := `{"schema_version":1,"key":"first"}`
	records, err := readRunHistory(strings.NewReader(valid + "\n" + `{"schema_version":1,"key":"partial`))
	if err != nil || len(records) != 1 || records[0].Key != "first" {
		t.Fatalf("records=%+v err=%v", records, err)
	}

	_, err = readRunHistory(strings.NewReader(`{"key":"broken` + "\n" + valid + "\n"))
	if err == nil {
		t.Fatal("malformed non-trailing record was accepted")
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("error=%T %v, want JSON syntax error", err, err)
	}
}

func TestTrimInvalidTrailingRunHistory(t *testing.T) {
	valid := []byte("{\"key\":\"first\"}\n")
	damaged := append(append([]byte(nil), valid...), []byte("{\"key\":\"partial")...)
	if got := trimInvalidTrailingRunHistory(damaged); string(got) != string(valid) {
		t.Fatalf("trimmed=%q, want %q", got, valid)
	}
	if got := trimInvalidTrailingRunHistory(valid); string(got) != string(valid) {
		t.Fatalf("valid history changed: %q", got)
	}
}

func TestAppendRunHistoryRecordsTerminalTaskAndDedupes(t *testing.T) {
	_, st := executionFixture(t)
	start := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	done := start.Add(2*time.Minute + 3*time.Second)
	state := RunState{
		RunID: "run-1",
		Study: st.Name,
		Config: ConfigSummary{
			Runtime: "opencode",
			Model:   "fallback-model",
		},
	}
	task := TaskState{
		ID:           "analysis-demo-01-repo",
		Kind:         TaskKindAnalysis,
		Status:       TaskStatusCompleted,
		Study:        st.Name,
		Dimension:    "01",
		DimensionRef: "01-structure",
		Source:       "repo",
		SourceKind:   SourceKindDirectory,
		OutputPath:   "studies/demo/reports/source/01-structure/repo.md",
		Attempts:     1,
		StartedAt:    &start,
		CompletedAt:  &done,
		Agent: AgentMetadata{
			Runtime:  "opencode",
			Provider: "minimax",
			Model:    "minimax-m3",
			RunID:    "agent-run-1",
			Usage: UsageMetadata{
				TotalTokensKnown: true,
				TotalTokens:      42,
			},
			Cost: &CostMetadata{Amount: 0.125, Currency: "USD", Estimate: true},
		},
		Validation: &ValidationSummary{Status: ValidationStatusPassed, PassedChecks: 5},
	}

	if err := AppendRunHistory(st, state, task); err != nil {
		t.Fatal(err)
	}
	if err := AppendRunHistory(st, state, task); err != nil {
		t.Fatal(err)
	}
	records, err := LoadRunHistory(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	record := records[0]
	if record.Provider != "minimax" || record.Model != "minimax-m3" || record.TotalTokens != 42 || !record.CostKnown {
		t.Fatalf("record metadata = %#v", record)
	}
	if record.DurationMS != 123000 {
		t.Fatalf("DurationMS = %d, want 123000", record.DurationMS)
	}
}

func TestRunLoopWritesRunHistoryAndSummary(t *testing.T) {
	root, st := executionFixture(t)
	rt := &runAllRuntime{write: validSourceReport}
	service := NewService(root, WithRuntime(rt, runtimeRequest()))

	if _, err := service.RunLoop(context.Background(), RunLoopRequest{
		StudyRef:      "demo",
		DimensionRefs: []string{"01"},
		SourceRefs:    []string{"repo"},
		Parallelism:   1,
		Config:        ConfigSummary{Runtime: "opencode", Model: "test-model"},
	}); err != nil {
		t.Fatal(err)
	}
	records, err := LoadRunHistory(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want filtered analysis only", len(records))
	}
	content, err := os.ReadFile(RunHistorySummaryPath(st))
	if err != nil {
		t.Fatal(err)
	}
	summary := string(content)
	for _, want := range []string{"# Study Run Summary", "## Dimensions", "01-structure", "repo"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q:\n%s", want, summary)
		}
	}
}
