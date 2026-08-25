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
	Model                      string `json:"model"`
	Variant                    string `json:"variant"`
	ChangedPaths               int    `json:"changed_paths"`
	PrimaryShards              int    `json:"primary_shards"`
	BoundaryShards             int    `json:"boundary_shards"`
	FollowUpShards             int    `json:"follow_up_shards"`
	TotalShards                int    `json:"total_shards"`
	PendingEntries             int    `json:"pending_entries"`
	ChangedPathsPerShard       int    `json:"changed_paths_per_shard"`
	ContextPathsPerShard       int    `json:"context_paths_per_shard"`
	ContextExpansions          int    `json:"context_expansions"`
	PathsPerExpansion          int    `json:"paths_per_expansion"`
	BehavioralConcernsPerShard int    `json:"behavioral_concerns_per_shard"`
	TheoriesPerShard           int    `json:"theories_per_shard"`
	IterationsPerAttempt       int    `json:"iterations_per_attempt"`
	CommandsPerAttempt         int    `json:"commands_per_attempt"`
	RuntimeRetries             int    `json:"runtime_retries"`
	ConcurrentInvestigators    int    `json:"concurrent_investigators"`
	CommandTimeout             string `json:"command_timeout"`
	ShardTimeout               string `json:"shard_timeout"`
	RunTimeout                 string `json:"run_timeout"`
	CleanupTimeout             string `json:"cleanup_timeout"`
	CommandOutputBytes         int    `json:"command_output_bytes"`
	ShardOutputBytes           int    `json:"shard_output_bytes"`
	PromptBytes                int    `json:"prompt_bytes"`
	RecentProgress             int    `json:"recent_progress"`
	RetainedAttempts           int    `json:"retained_attempts"`
	StateBytes                 int    `json:"state_bytes"`
}

func DefaultQA() QA {
	return QA{
		ChangedPaths: 512, PrimaryShards: 32, BoundaryShards: 8,
		FollowUpShards: 4, TotalShards: 44, PendingEntries: 44,
		ChangedPathsPerShard: 32, ContextPathsPerShard: 64,
		ContextExpansions: 2, PathsPerExpansion: 16,
		BehavioralConcernsPerShard: 12, TheoriesPerShard: 12,
		IterationsPerAttempt: 4, CommandsPerAttempt: 8, RuntimeRetries: 1,
		ConcurrentInvestigators: 3, CommandTimeout: "5m", ShardTimeout: "20m",
		RunTimeout: "60m", CleanupTimeout: "30s", CommandOutputBytes: 256 << 10,
		ShardOutputBytes: 1 << 20, PromptBytes: 512 << 10, RecentProgress: 100,
		RetainedAttempts: 8, StateBytes: 128 << 20,
	}
}

func maxQA() QA {
	return QA{
		ChangedPaths: 512, PrimaryShards: 32, BoundaryShards: 8,
		FollowUpShards: 4, TotalShards: 44, PendingEntries: 44,
		ChangedPathsPerShard: 64, ContextPathsPerShard: 128,
		ContextExpansions: 4, PathsPerExpansion: 32,
		BehavioralConcernsPerShard: 24, TheoriesPerShard: 24,
		IterationsPerAttempt: 8, CommandsPerAttempt: 16, RuntimeRetries: 2,
		ConcurrentInvestigators: 8, CommandTimeout: "10m", ShardTimeout: "30m",
		RunTimeout: "90m", CleanupTimeout: "30s", CommandOutputBytes: 512 << 10,
		ShardOutputBytes: 2 << 20, PromptBytes: 1 << 20, RecentProgress: 200,
		RetainedAttempts: 8, StateBytes: 128 << 20,
	}
}

func qaConfigFields() []string {
	return []string{
		"qa.model", "qa.variant", "qa.changed_paths", "qa.primary_shards",
		"qa.boundary_shards", "qa.follow_up_shards", "qa.total_shards",
		"qa.pending_entries", "qa.changed_paths_per_shard",
		"qa.context_paths_per_shard", "qa.context_expansions",
		"qa.paths_per_expansion", "qa.behavioral_concerns_per_shard",
		"qa.theories_per_shard", "qa.iterations_per_attempt",
		"qa.commands_per_attempt", "qa.runtime_retries",
		"qa.concurrent_investigators", "qa.command_timeout", "qa.shard_timeout",
		"qa.run_timeout", "qa.cleanup_timeout", "qa.command_output_bytes",
		"qa.shard_output_bytes", "qa.prompt_bytes", "qa.recent_progress",
		"qa.retained_attempts", "qa.state_bytes",
	}
}

func QAConfigFields() []string {
	return append([]string(nil), qaConfigFields()...)
}

func qaEnvOverrides() []EnvOverride {
	fields := qaConfigFields()
	overrides := make([]EnvOverride, 0, len(fields))
	for _, field := range fields {
		suffix := strings.TrimPrefix(field, "qa.")
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
	case "qa.runtime_retries":
		return setQAInteger(field, value, &q.RuntimeRetries)
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
	default:
		return false, nil
	}
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
		{"runtime_retries", q.RuntimeRetries, max.RuntimeRetries}, {"concurrent_investigators", q.ConcurrentInvestigators, max.ConcurrentInvestigators},
		{"command_output_bytes", q.CommandOutputBytes, max.CommandOutputBytes}, {"shard_output_bytes", q.ShardOutputBytes, max.ShardOutputBytes},
		{"prompt_bytes", q.PromptBytes, max.PromptBytes}, {"recent_progress", q.RecentProgress, max.RecentProgress},
		{"retained_attempts", q.RetainedAttempts, max.RetainedAttempts}, {"state_bytes", q.StateBytes, max.StateBytes},
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
	}{{"command_timeout", q.CommandTimeout, max.CommandTimeout}, {"shard_timeout", q.ShardTimeout, max.ShardTimeout}, {"run_timeout", q.RunTimeout, max.RunTimeout}, {"cleanup_timeout", q.CleanupTimeout, max.CleanupTimeout}}
	for _, limit := range durations {
		got, err := time.ParseDuration(limit.got)
		maximum, _ := time.ParseDuration(limit.max)
		if err != nil || got <= 0 || got > maximum {
			return fmt.Errorf("qa.%s: must be a positive duration no greater than %s", limit.name, limit.max)
		}
	}
	return nil
}
