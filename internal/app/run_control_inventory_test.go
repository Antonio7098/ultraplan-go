package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEveryRuntimeBackedCLIEntryUsesDurableAcceptanceInventory(t *testing.T) {
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source")
	}
	directory := filepath.Dir(current)
	cases := []struct {
		file     string
		wantCall int
		entries  []string
	}{
		{
			file: "sprint_commands.go", wantCall: 8,
			entries: []string{
				"Kind: OperationFlow", "Kind: OperationVerifyStart", "Kind: OperationExecuteStart",
				"Kind: OperationReviewStart", "Kind: OperationSmokeStart",
				"kind := OperationQAStart", "kind = OperationQAResume",
				"Kind: OperationRepairPrepare", "Kind: OperationRepairStart", "Kind: OperationRepairResume", "Kind: OperationRepairRecover",
				"Kind: OperationRepairCampaignStart",
			},
		},
		{
			file: "study_commands.go", wantCall: 4,
			entries: []string{
				"Kind: OperationStudyResume", "Kind: OperationStudyStart, Study: studyRef, Parallelism:",
				`Stage: "analysis"`, `Stage: "synthesis"`,
			},
		},
	}
	for _, test := range cases {
		t.Run(test.file, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join(directory, test.file))
			if err != nil {
				t.Fatal(err)
			}
			source := string(content)
			if got := strings.Count(source, "beginDurableCLICommand("); got != test.wantCall {
				t.Fatalf("durable entry calls=%d, want %d", got, test.wantCall)
			}
			for _, entry := range test.entries {
				if !strings.Contains(source, entry) {
					t.Fatalf("runtime-backed inventory entry %q bypasses the durable boundary", entry)
				}
			}
		})
	}
}
