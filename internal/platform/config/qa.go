package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// QA contains workspace-selected read-only QA policy. Every limit is
// lower-only: configuration may reduce a product default but cannot exceed the
// shipped maximum.
type QA struct {
	Model                      string   `json:"model"`
	Variant                    string   `json:"variant"`
	MapperModel                string   `json:"mapper_model"`
	MapperVariant              string   `json:"mapper_variant"`
	InvestigatorModel          string   `json:"investigator_model"`
	InvestigatorVariant        string   `json:"investigator_variant"`
	ChallengerModel            string   `json:"challenger_model"`
	ChallengerVariant          string   `json:"challenger_variant"`
	ArbiterModel               string   `json:"arbiter_model"`
	ArbiterVariant             string   `json:"arbiter_variant"`
	ReconcilerModel            string   `json:"reconciler_model"`
	ReconcilerVariant          string   `json:"reconciler_variant"`
	EvaluatorModel             string   `json:"evaluator_model"`
	EvaluatorVariant           string   `json:"evaluator_variant"`
	RepairModel                string   `json:"repair_model"`
	RepairVariant              string   `json:"repair_variant"`
	ArbiterMaxTheories         int      `json:"arbiter_max_theories"`
	RepairAssignmentMode       string   `json:"repair_assignment_mode"`
	IssuesPerRepairAgent       int      `json:"issues_per_repair_agent"`
	RepairExecutionMode        string   `json:"repair_execution_mode"`
	ChangedPaths               int      `json:"changed_paths"`
	PrimaryShards              int      `json:"primary_shards"`
	BoundaryShards             int      `json:"boundary_shards"`
	FollowUpShards             int      `json:"follow_up_shards"`
	TotalShards                int      `json:"total_shards"`
	PendingEntries             int      `json:"pending_entries"`
	ChangedPathsPerShard       int      `json:"changed_paths_per_shard"`
	ContextPathsPerShard       int      `json:"context_paths_per_shard"`
	ContextExpansions          int      `json:"context_expansions"`
	PathsPerExpansion          int      `json:"paths_per_expansion"`
	BehavioralConcernsPerShard int      `json:"behavioral_concerns_per_shard"`
	TheoriesPerShard           int      `json:"theories_per_shard"`
	IterationsPerAttempt       int      `json:"iterations_per_attempt"`
	CommandsPerAttempt         int      `json:"commands_per_attempt"`
	OutputRepairAttempts       int      `json:"output_repair_attempts"`
	ConcurrentInvestigators    int      `json:"concurrent_investigators"`
	CommandTimeout             string   `json:"command_timeout"`
	ShardTimeout               string   `json:"shard_timeout"`
	RunTimeout                 string   `json:"run_timeout"`
	CleanupTimeout             string   `json:"cleanup_timeout"`
	CommandOutputBytes         int      `json:"command_output_bytes"`
	ShardOutputBytes           int      `json:"shard_output_bytes"`
	PromptBytes                int      `json:"prompt_bytes"`
	RecentProgress             int      `json:"recent_progress"`
	RetainedAttempts           int      `json:"retained_attempts"`
	StateBytes                 int      `json:"state_bytes"`
	TreeFiles                  int      `json:"tree_files"`
	TreeBytes                  int      `json:"tree_bytes"`
	FileBytes                  int      `json:"file_bytes"`
	GeneratedChecks            int      `json:"generated_checks"`
	GeneratedPatchBytes        int      `json:"generated_patch_bytes"`
	EvidenceRecords            int      `json:"evidence_records"`
	Issues                     int      `json:"issues"`
	EvidenceRoundsPerShard     int      `json:"evidence_rounds_per_shard"`
	TestsPerTheory             int      `json:"tests_per_theory"`
	TestsPerIssue              int      `json:"tests_per_issue"`
	AuthoredTestFiles          int      `json:"authored_test_files"`
	AuthoredTestBytes          int      `json:"authored_test_bytes"`
	TestCommandsPerRound       int      `json:"test_commands_per_round"`
	AuthoringRuntimeTurns      int      `json:"authoring_runtime_turns"`
	AuthoringWallTime          string   `json:"authoring_wall_time"`
	Repair                     QARepair `json:"repair"`
}

// QARepair contains lower-only bounded-repair policy. Product code owns the
// immutable maxima; configuration can only reduce these defaults.
type QARepair struct {
	MaxCycles         int    `json:"max_cycles"`
	MaxMutationCycles int    `json:"max_mutation_cycles"`
	MaxReopenings     int    `json:"max_reopenings"`
	StagnationLimit   int    `json:"stagnation_limit"`
	MaxFilesPerCycle  int    `json:"max_files_per_cycle"`
	MaxFilesPerRun    int    `json:"max_files_per_run"`
	MaxBytesPerCycle  int64  `json:"max_bytes_per_cycle"`
	MaxBytesPerRun    int64  `json:"max_bytes_per_run"`
	MaxPatchBytes     int    `json:"max_patch_bytes"`
	WallTime          string `json:"wall_time"`
	RuntimeAttempts   int    `json:"runtime_attempts"`
	ModelTurns        int    `json:"model_turns"`
	CommandCount      int    `json:"command_count"`
	CommandTimeout    string `json:"command_timeout"`
	OutputBytes       int    `json:"output_bytes"`
	RetainedCycles    int    `json:"retained_cycles"`
	CleanupTimeout    string `json:"cleanup_timeout"`
}

func DefaultQA() QA {
	return QA{
		ArbiterMaxTheories: 24, RepairAssignmentMode: "per_issue", IssuesPerRepairAgent: 1, RepairExecutionMode: "sequential",
		ChangedPaths: 512, PrimaryShards: 32, BoundaryShards: 8,
		FollowUpShards: 4, TotalShards: 44, PendingEntries: 44,
		ChangedPathsPerShard: 12, ContextPathsPerShard: 64,
		ContextExpansions: 2, PathsPerExpansion: 16,
		BehavioralConcernsPerShard: 12, TheoriesPerShard: 12,
		IterationsPerAttempt: 4, CommandsPerAttempt: 8, OutputRepairAttempts: 1,
		ConcurrentInvestigators: 3, CommandTimeout: "5m", ShardTimeout: "20m",
		RunTimeout: "60m", CleanupTimeout: "30s", CommandOutputBytes: 256 << 10,
		ShardOutputBytes: 1 << 20, PromptBytes: 512 << 10, RecentProgress: 100,
		RetainedAttempts: 8, StateBytes: 128 << 20,
		TreeFiles: 200_000, TreeBytes: 2 << 30, FileBytes: 32 << 20,
		GeneratedChecks: 88, GeneratedPatchBytes: 2 << 20, EvidenceRecords: 256, Issues: 200,
		EvidenceRoundsPerShard: 2, TestsPerTheory: 2, TestsPerIssue: 8, AuthoredTestFiles: 8,
		AuthoredTestBytes: 256 << 10, TestCommandsPerRound: 4, AuthoringRuntimeTurns: 4, AuthoringWallTime: "10m",
		Repair: QARepair{MaxCycles: 3, MaxMutationCycles: 3, MaxReopenings: 1, StagnationLimit: 1, MaxFilesPerCycle: 8, MaxFilesPerRun: 16, MaxBytesPerCycle: 256 << 10, MaxBytesPerRun: 512 << 10, MaxPatchBytes: 512 << 10, WallTime: "45m", RuntimeAttempts: 3, ModelTurns: 12, CommandCount: 32, CommandTimeout: "10m", OutputBytes: 1 << 20, RetainedCycles: 8, CleanupTimeout: "30s"},
	}
}

func maxQA() QA {
	return QA{
		ArbiterMaxTheories: 64, RepairAssignmentMode: "grouped", IssuesPerRepairAgent: 16, RepairExecutionMode: "parallel",
		ChangedPaths: 512, PrimaryShards: 32, BoundaryShards: 8,
		FollowUpShards: 4, TotalShards: 44, PendingEntries: 44,
		ChangedPathsPerShard: 64, ContextPathsPerShard: 128,
		ContextExpansions: 4, PathsPerExpansion: 32,
		BehavioralConcernsPerShard: 24, TheoriesPerShard: 24,
		IterationsPerAttempt: 8, CommandsPerAttempt: 16, OutputRepairAttempts: 2,
		ConcurrentInvestigators: 8, CommandTimeout: "10m", ShardTimeout: "30m",
		RunTimeout: "90m", CleanupTimeout: "30s", CommandOutputBytes: 512 << 10,
		ShardOutputBytes: 2 << 20, PromptBytes: 1 << 20, RecentProgress: 200,
		RetainedAttempts: 8, StateBytes: 128 << 20,
		TreeFiles: 400_000, TreeBytes: 4 << 30, FileBytes: 64 << 20,
		GeneratedChecks: 128, GeneratedPatchBytes: 4 << 20, EvidenceRecords: 512, Issues: 200,
		EvidenceRoundsPerShard: 4, TestsPerTheory: 4, TestsPerIssue: 16, AuthoredTestFiles: 16,
		AuthoredTestBytes: 1 << 20, TestCommandsPerRound: 8, AuthoringRuntimeTurns: 8, AuthoringWallTime: "20m",
	}
}

func qaConfigFields() []string {
	return []string{
		"qa.model", "qa.variant", "qa.mapper_model", "qa.mapper_variant", "qa.investigator_model", "qa.investigator_variant",
		"qa.challenger_model", "qa.challenger_variant", "qa.evaluator_model", "qa.evaluator_variant",
		"qa.arbiter_model", "qa.arbiter_variant", "qa.reconciler_model", "qa.reconciler_variant",
		"qa.repair_model", "qa.repair_variant", "qa.arbiter_max_theories", "qa.repair_assignment_mode", "qa.issues_per_repair_agent", "qa.repair_execution_mode",
		"qa.changed_paths", "qa.primary_shards",
		"qa.boundary_shards", "qa.follow_up_shards", "qa.total_shards",
		"qa.pending_entries", "qa.changed_paths_per_shard",
		"qa.context_paths_per_shard", "qa.context_expansions",
		"qa.paths_per_expansion", "qa.behavioral_concerns_per_shard",
		"qa.theories_per_shard", "qa.iterations_per_attempt",
		"qa.commands_per_attempt", "qa.output_repair_attempts",
		"qa.concurrent_investigators", "qa.command_timeout", "qa.shard_timeout",
		"qa.run_timeout", "qa.cleanup_timeout", "qa.command_output_bytes",
		"qa.shard_output_bytes", "qa.prompt_bytes", "qa.recent_progress",
		"qa.retained_attempts", "qa.state_bytes",
		"qa.tree_files", "qa.tree_bytes", "qa.file_bytes", "qa.generated_checks",
		"qa.generated_patch_bytes", "qa.evidence_records", "qa.issues",
		"qa.evidence_rounds_per_shard", "qa.tests_per_theory", "qa.tests_per_issue", "qa.authored_test_files",
		"qa.authored_test_bytes", "qa.test_commands_per_round", "qa.authoring_runtime_turns", "qa.authoring_wall_time",
		"qa.repair.max_cycles", "qa.repair.max_mutation_cycles", "qa.repair.max_reopenings", "qa.repair.stagnation_limit",
		"qa.repair.max_files_per_cycle", "qa.repair.max_files_per_run", "qa.repair.max_bytes_per_cycle", "qa.repair.max_bytes_per_run",
		"qa.repair.max_patch_bytes", "qa.repair.wall_time", "qa.repair.runtime_attempts", "qa.repair.model_turns",
		"qa.repair.command_count", "qa.repair.command_timeout", "qa.repair.output_bytes", "qa.repair.retained_cycles", "qa.repair.cleanup_timeout",
	}
}

func QAConfigFields() []string {
	return append([]string(nil), qaConfigFields()...)
}

func qaEnvOverrides() []EnvOverride {
	fields := qaConfigFields()
	overrides := make([]EnvOverride, 0, len(fields))
	for _, field := range fields {
		suffix := strings.ReplaceAll(strings.TrimPrefix(field, "qa."), ".", "_")
		overrides = append(overrides, EnvOverride{
			Key:   "ULTRAPLAN_QA_" + strings.ToUpper(suffix),
			Field: field,
		})
	}
	return overrides
}

func setQAField(q *QA, field, value string) (bool, error) {
	if !strings.HasPrefix(field, "qa.") {
		return false, nil
	}
	if field == "qa.model" {
		q.Model = value
		return true, nil
	}
	if field == "qa.variant" {
		q.Variant = value
		return true, nil
	}
	stageStrings := map[string]*string{
		"qa.mapper_model": &q.MapperModel, "qa.mapper_variant": &q.MapperVariant,
		"qa.investigator_model": &q.InvestigatorModel, "qa.investigator_variant": &q.InvestigatorVariant,
		"qa.challenger_model": &q.ChallengerModel, "qa.challenger_variant": &q.ChallengerVariant,
		"qa.arbiter_model": &q.ArbiterModel, "qa.arbiter_variant": &q.ArbiterVariant,
		"qa.reconciler_model": &q.ReconcilerModel, "qa.reconciler_variant": &q.ReconcilerVariant,
		"qa.evaluator_model": &q.EvaluatorModel, "qa.evaluator_variant": &q.EvaluatorVariant,
		"qa.repair_model": &q.RepairModel, "qa.repair_variant": &q.RepairVariant,
	}
	if target, ok := stageStrings[field]; ok {
		*target = value
		return true, nil
	}
	if field == "qa.repair_assignment_mode" {
		q.RepairAssignmentMode = value
		return true, nil
	}
	if field == "qa.repair_execution_mode" {
		q.RepairExecutionMode = value
		return true, nil
	}
	if field == "qa.command_timeout" {
		q.CommandTimeout = value
		return true, nil
	}
	if field == "qa.shard_timeout" {
		q.ShardTimeout = value
		return true, nil
	}
	if field == "qa.run_timeout" {
		q.RunTimeout = value
		return true, nil
	}
	if field == "qa.cleanup_timeout" {
		q.CleanupTimeout = value
		return true, nil
	}
	if field == "qa.authoring_wall_time" {
		q.AuthoringWallTime = value
		return true, nil
	}
	if field == "qa.repair.wall_time" {
		q.Repair.WallTime = value
		return true, nil
	}
	if field == "qa.repair.command_timeout" {
		q.Repair.CommandTimeout = value
		return true, nil
	}
	if field == "qa.repair.cleanup_timeout" {
		q.Repair.CleanupTimeout = value
		return true, nil
	}
	switch field {
	case "qa.changed_paths":
		return setQAInteger(field, value, &q.ChangedPaths)
	case "qa.primary_shards":
		return setQAInteger(field, value, &q.PrimaryShards)
	case "qa.boundary_shards":
		return setQAInteger(field, value, &q.BoundaryShards)
	case "qa.follow_up_shards":
		return setQAInteger(field, value, &q.FollowUpShards)
	case "qa.total_shards":
		return setQAInteger(field, value, &q.TotalShards)
	case "qa.pending_entries":
		return setQAInteger(field, value, &q.PendingEntries)
	case "qa.changed_paths_per_shard":
		return setQAInteger(field, value, &q.ChangedPathsPerShard)
	case "qa.context_paths_per_shard":
		return setQAInteger(field, value, &q.ContextPathsPerShard)
	case "qa.context_expansions":
		return setQAInteger(field, value, &q.ContextExpansions)
	case "qa.paths_per_expansion":
		return setQAInteger(field, value, &q.PathsPerExpansion)
	case "qa.behavioral_concerns_per_shard":
		return setQAInteger(field, value, &q.BehavioralConcernsPerShard)
	case "qa.theories_per_shard":
		return setQAInteger(field, value, &q.TheoriesPerShard)
	case "qa.iterations_per_attempt":
		return setQAInteger(field, value, &q.IterationsPerAttempt)
	case "qa.commands_per_attempt":
		return setQAInteger(field, value, &q.CommandsPerAttempt)
	case "qa.output_repair_attempts":
		return setQAInteger(field, value, &q.OutputRepairAttempts)
	case "qa.concurrent_investigators":
		return setQAInteger(field, value, &q.ConcurrentInvestigators)
	case "qa.command_output_bytes":
		return setQAInteger(field, value, &q.CommandOutputBytes)
	case "qa.shard_output_bytes":
		return setQAInteger(field, value, &q.ShardOutputBytes)
	case "qa.prompt_bytes":
		return setQAInteger(field, value, &q.PromptBytes)
	case "qa.recent_progress":
		return setQAInteger(field, value, &q.RecentProgress)
	case "qa.retained_attempts":
		return setQAInteger(field, value, &q.RetainedAttempts)
	case "qa.state_bytes":
		return setQAInteger(field, value, &q.StateBytes)
	case "qa.tree_files":
		return setQAInteger(field, value, &q.TreeFiles)
	case "qa.tree_bytes":
		return setQAInteger(field, value, &q.TreeBytes)
	case "qa.file_bytes":
		return setQAInteger(field, value, &q.FileBytes)
	case "qa.generated_checks":
		return setQAInteger(field, value, &q.GeneratedChecks)
	case "qa.generated_patch_bytes":
		return setQAInteger(field, value, &q.GeneratedPatchBytes)
	case "qa.evidence_records":
		return setQAInteger(field, value, &q.EvidenceRecords)
	case "qa.issues":
		return setQAInteger(field, value, &q.Issues)
	case "qa.evidence_rounds_per_shard":
		return setQAInteger(field, value, &q.EvidenceRoundsPerShard)
	case "qa.tests_per_theory":
		return setQAInteger(field, value, &q.TestsPerTheory)
	case "qa.tests_per_issue":
		return setQAInteger(field, value, &q.TestsPerIssue)
	case "qa.authored_test_files":
		return setQAInteger(field, value, &q.AuthoredTestFiles)
	case "qa.authored_test_bytes":
		return setQAInteger(field, value, &q.AuthoredTestBytes)
	case "qa.test_commands_per_round":
		return setQAInteger(field, value, &q.TestCommandsPerRound)
	case "qa.authoring_runtime_turns":
		return setQAInteger(field, value, &q.AuthoringRuntimeTurns)
	case "qa.issues_per_repair_agent":
		return setQAInteger(field, value, &q.IssuesPerRepairAgent)
	case "qa.arbiter_max_theories":
		return setQAInteger(field, value, &q.ArbiterMaxTheories)
	case "qa.repair.max_cycles":
		return setQAInteger(field, value, &q.Repair.MaxCycles)
	case "qa.repair.max_mutation_cycles":
		return setQAInteger(field, value, &q.Repair.MaxMutationCycles)
	case "qa.repair.max_reopenings":
		return setQAInteger(field, value, &q.Repair.MaxReopenings)
	case "qa.repair.stagnation_limit":
		return setQAInteger(field, value, &q.Repair.StagnationLimit)
	case "qa.repair.max_files_per_cycle":
		return setQAInteger(field, value, &q.Repair.MaxFilesPerCycle)
	case "qa.repair.max_files_per_run":
		return setQAInteger(field, value, &q.Repair.MaxFilesPerRun)
	case "qa.repair.max_bytes_per_cycle":
		return setQAInt64(field, value, &q.Repair.MaxBytesPerCycle)
	case "qa.repair.max_bytes_per_run":
		return setQAInt64(field, value, &q.Repair.MaxBytesPerRun)
	case "qa.repair.max_patch_bytes":
		return setQAInteger(field, value, &q.Repair.MaxPatchBytes)
	case "qa.repair.runtime_attempts":
		return setQAInteger(field, value, &q.Repair.RuntimeAttempts)
	case "qa.repair.model_turns":
		return setQAInteger(field, value, &q.Repair.ModelTurns)
	case "qa.repair.command_count":
		return setQAInteger(field, value, &q.Repair.CommandCount)
	case "qa.repair.output_bytes":
		return setQAInteger(field, value, &q.Repair.OutputBytes)
	case "qa.repair.retained_cycles":
		return setQAInteger(field, value, &q.Repair.RetainedCycles)
	default:
		return false, nil
	}
}

func setQAInt64(field, value string, target *int64) (bool, error) {
	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return true, fmt.Errorf("%s: must be an integer", field)
	}
	*target = n
	return true, nil
}

func setQAInteger(field, value string, target *int) (bool, error) {
	n, err := strconv.Atoi(value)
	if err != nil {
		return true, fmt.Errorf("%s: must be an integer", field)
	}
	*target = n
	return true, nil
}

func validateQA(q QA) error {
	max := maxQA()
	if q.RepairAssignmentMode != "per_issue" && q.RepairAssignmentMode != "grouped" {
		return fmt.Errorf("qa.repair_assignment_mode: must be per_issue or grouped")
	}
	if q.RepairExecutionMode != "sequential" && q.RepairExecutionMode != "parallel" {
		return fmt.Errorf("qa.repair_execution_mode: must be sequential or parallel")
	}
	if q.ArbiterMaxTheories <= 0 || q.ArbiterMaxTheories > max.ArbiterMaxTheories {
		return fmt.Errorf("qa.arbiter_max_theories: must be between 1 and %d", max.ArbiterMaxTheories)
	}
	if q.IssuesPerRepairAgent <= 0 || q.IssuesPerRepairAgent > max.IssuesPerRepairAgent {
		return fmt.Errorf("qa.issues_per_repair_agent: must be between 1 and %d", max.IssuesPerRepairAgent)
	}
	if q.RepairAssignmentMode == "per_issue" && q.IssuesPerRepairAgent != 1 {
		return fmt.Errorf("qa.issues_per_repair_agent: must be 1 in per_issue mode")
	}
	limits := []struct {
		name string
		got  int
		max  int
	}{
		{"changed_paths", q.ChangedPaths, max.ChangedPaths}, {"primary_shards", q.PrimaryShards, max.PrimaryShards},
		{"boundary_shards", q.BoundaryShards, max.BoundaryShards}, {"follow_up_shards", q.FollowUpShards, max.FollowUpShards},
		{"total_shards", q.TotalShards, max.TotalShards}, {"pending_entries", q.PendingEntries, max.PendingEntries},
		{"changed_paths_per_shard", q.ChangedPathsPerShard, max.ChangedPathsPerShard}, {"context_paths_per_shard", q.ContextPathsPerShard, max.ContextPathsPerShard},
		{"context_expansions", q.ContextExpansions, max.ContextExpansions}, {"paths_per_expansion", q.PathsPerExpansion, max.PathsPerExpansion},
		{"behavioral_concerns_per_shard", q.BehavioralConcernsPerShard, max.BehavioralConcernsPerShard}, {"theories_per_shard", q.TheoriesPerShard, max.TheoriesPerShard},
		{"iterations_per_attempt", q.IterationsPerAttempt, max.IterationsPerAttempt}, {"commands_per_attempt", q.CommandsPerAttempt, max.CommandsPerAttempt},
		{"output_repair_attempts", q.OutputRepairAttempts, max.OutputRepairAttempts}, {"concurrent_investigators", q.ConcurrentInvestigators, max.ConcurrentInvestigators},
		{"command_output_bytes", q.CommandOutputBytes, max.CommandOutputBytes}, {"shard_output_bytes", q.ShardOutputBytes, max.ShardOutputBytes},
		{"prompt_bytes", q.PromptBytes, max.PromptBytes}, {"recent_progress", q.RecentProgress, max.RecentProgress},
		{"retained_attempts", q.RetainedAttempts, max.RetainedAttempts}, {"state_bytes", q.StateBytes, max.StateBytes},
		{"tree_files", q.TreeFiles, max.TreeFiles}, {"tree_bytes", q.TreeBytes, max.TreeBytes}, {"file_bytes", q.FileBytes, max.FileBytes},
		{"generated_checks", q.GeneratedChecks, max.GeneratedChecks}, {"generated_patch_bytes", q.GeneratedPatchBytes, max.GeneratedPatchBytes},
		{"evidence_records", q.EvidenceRecords, max.EvidenceRecords}, {"issues", q.Issues, max.Issues},
		{"evidence_rounds_per_shard", q.EvidenceRoundsPerShard, max.EvidenceRoundsPerShard}, {"tests_per_theory", q.TestsPerTheory, max.TestsPerTheory},
		{"tests_per_issue", q.TestsPerIssue, max.TestsPerIssue}, {"authored_test_files", q.AuthoredTestFiles, max.AuthoredTestFiles},
		{"authored_test_bytes", q.AuthoredTestBytes, max.AuthoredTestBytes}, {"test_commands_per_round", q.TestCommandsPerRound, max.TestCommandsPerRound},
		{"authoring_runtime_turns", q.AuthoringRuntimeTurns, max.AuthoringRuntimeTurns},
	}
	for _, limit := range limits {
		if limit.got <= 0 || limit.got > limit.max {
			return fmt.Errorf("qa.%s: must be between 1 and %d", limit.name, limit.max)
		}
	}
	durations := []struct {
		name string
		got  string
		max  string
	}{{"command_timeout", q.CommandTimeout, max.CommandTimeout}, {"shard_timeout", q.ShardTimeout, max.ShardTimeout}, {"run_timeout", q.RunTimeout, max.RunTimeout}, {"cleanup_timeout", q.CleanupTimeout, max.CleanupTimeout}, {"authoring_wall_time", q.AuthoringWallTime, max.AuthoringWallTime}}
	for _, limit := range durations {
		got, err := time.ParseDuration(limit.got)
		maximum, _ := time.ParseDuration(limit.max)
		if err != nil || got <= 0 || got > maximum {
			return fmt.Errorf("qa.%s: must be a positive duration no greater than %s", limit.name, limit.max)
		}
	}
	repairInts := []struct {
		name string
		got  int
		max  int
	}{
		{"max_cycles", q.Repair.MaxCycles, 5}, {"max_mutation_cycles", q.Repair.MaxMutationCycles, 5}, {"max_reopenings", q.Repair.MaxReopenings, 2}, {"stagnation_limit", q.Repair.StagnationLimit, 2},
		{"max_files_per_cycle", q.Repair.MaxFilesPerCycle, 16}, {"max_files_per_run", q.Repair.MaxFilesPerRun, 32}, {"max_patch_bytes", q.Repair.MaxPatchBytes, 1 << 20},
		{"runtime_attempts", q.Repair.RuntimeAttempts, 5}, {"model_turns", q.Repair.ModelTurns, 20}, {"command_count", q.Repair.CommandCount, 64}, {"output_bytes", q.Repair.OutputBytes, 2 << 20}, {"retained_cycles", q.Repair.RetainedCycles, 12},
	}
	for _, limit := range repairInts {
		if limit.got <= 0 || limit.got > limit.max {
			return fmt.Errorf("qa.repair.%s: must be between 1 and %d", limit.name, limit.max)
		}
	}
	if q.Repair.MaxMutationCycles > q.Repair.MaxCycles || q.Repair.MaxFilesPerCycle > q.Repair.MaxFilesPerRun || q.Repair.MaxBytesPerCycle <= 0 || q.Repair.MaxBytesPerCycle > 1<<20 || q.Repair.MaxBytesPerRun <= 0 || q.Repair.MaxBytesPerRun > 2<<20 || q.Repair.MaxBytesPerCycle > q.Repair.MaxBytesPerRun {
		return fmt.Errorf("qa.repair: per-cycle limits must be positive and no greater than run limits")
	}
	for _, limit := range []struct{ name, got, max string }{{"wall_time", q.Repair.WallTime, "90m"}, {"command_timeout", q.Repair.CommandTimeout, "15m"}, {"cleanup_timeout", q.Repair.CleanupTimeout, "60s"}} {
		got, err := time.ParseDuration(limit.got)
		maximum, _ := time.ParseDuration(limit.max)
		if err != nil || got <= 0 || got > maximum {
			return fmt.Errorf("qa.repair.%s: must be a positive duration no greater than %s", limit.name, limit.max)
		}
	}
	return nil
}
