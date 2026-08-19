package app

import "testing"

func TestExecuteTerminalComplete(t *testing.T) {
	tests := []struct {
		name    string
		summary ExecuteSummary
		want    bool
	}{
		{name: "all complete", summary: ExecuteSummary{Available: true, Total: 3, Complete: 3}, want: true},
		{name: "accepted deferred work", summary: ExecuteSummary{Available: true, Total: 3, Complete: 2, Deferred: 1}, want: true},
		{name: "pending", summary: ExecuteSummary{Available: true, Total: 3, Complete: 2, Pending: 1}},
		{name: "failed", summary: ExecuteSummary{Available: true, Total: 3, Complete: 2, Failed: 1}},
		{name: "empty", summary: ExecuteSummary{Available: true}},
		{name: "unavailable", summary: ExecuteSummary{Total: 1, Complete: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := executeTerminalComplete(test.summary); got != test.want {
				t.Fatalf("executeTerminalComplete() = %t, want %t", got, test.want)
			}
		})
	}
}
