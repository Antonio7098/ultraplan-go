package app

import (
	"strings"
	"testing"
	"time"

	"github.com/Antonio7098/ultraplan-go/internal/sprint"
)

func TestParseSprintVerifySmokeAndFlowQA(t *testing.T) {
	req, jsonOut, err := parseSprintVerifyArgs([]string{"--to", "smoke", "--focus-review", "contract-errors", "--suite", "sprint-28", "--timeout", "2m", "--force-review", "--override-reason", "diagnostic evidence", "--yes", "--json"})
	if err != nil || !jsonOut || req.To != sprint.StageSmoke || len(req.Review.Focus) != 1 || req.Smoke.Suite != "sprint-28" || req.Smoke.Timeout != 2*time.Minute || !req.Smoke.OverrideConfirmed || req.Smoke.OverrideRationale == "" {
		t.Fatalf("request=%+v json=%t err=%v", req, jsonOut, err)
	}
	flow, err := parseSprintFlowArgs([]string{"--to", "qa"})
	if err != nil || flow.To != sprint.StageQA {
		t.Fatalf("flow=%+v err=%v", flow, err)
	}
	if _, err := parseSprintFlowArgs([]string{"--to", "smoke"}); err == nil {
		t.Fatal("expected smoke to be unavailable as a flow target")
	}
	if _, _, err := parseSprintVerifyArgs([]string{"--to", "plan"}); err == nil {
		t.Fatal("expected verify target validation")
	}
	if _, _, err := parseSprintVerifyArgs([]string{"--force-review", "--yes"}); err == nil {
		t.Fatal("expected override rationale validation")
	}
	restart, _, err := parseSprintVerifyArgs([]string{"--to", "review", "--restart-review"})
	if err != nil || !restart.Review.Restart {
		t.Fatalf("restart request=%+v err=%v", restart, err)
	}
	flowRestart, err := parseSprintFlowArgs([]string{"--to", "review", "--restart-review"})
	if err != nil || !flowRestart.Review.Restart {
		t.Fatalf("flow restart=%+v err=%v", flowRestart, err)
	}
	if _, _, err := parseSprintVerifyArgs([]string{"--restart-review", "--focus-review", "contract-errors"}); err == nil {
		t.Fatal("expected restart/focus conflict")
	}
}

func TestVerifyHelpExplainsGateAndRecovery(t *testing.T) {
	help := sprintHelp() + sprintVerifyHelp() + sprintFlowHelp()
	for _, want := range []string{"verify [--to review|smoke]", "--restart-review", "complete execute evidence", "current review", "containing", "override", "cannot improve", "--yes"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help missing %q", want)
		}
	}
}
