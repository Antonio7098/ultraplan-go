package app

import (
	"time"

	"ultraplan-go/internal/platform/config"
	"ultraplan-go/internal/study"
	"ultraplan-go/internal/workspace"
)

type statusJSON struct {
	SchemaVersion int                 `json:"schema_version"`
	RunID         string              `json:"run_id"`
	Complete      bool                `json:"complete"`
	StatePath     string              `json:"state_path"`
	Counts        statusCounts        `json:"counts"`
	Lock          *study.LockInfo     `json:"lock,omitempty"`
	RunMetadata   statusRunMetadata   `json:"run_metadata"`
	Tasks         []statusTaskJSON    `json:"tasks"`
	Usage         knownValue[int64]   `json:"usage"`
	Cost          knownValue[float64] `json:"cost"`
}

type statusCounts struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	Running    int `json:"running"`
	Validating int `json:"validating"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Cancelled  int `json:"cancelled"`
	Skipped    int `json:"skipped"`
	Waiting    int `json:"waiting"`
	Retrying   int `json:"retrying"`
	Active     int `json:"active"`
	Retries    int `json:"retries"`
}

type statusRunMetadata struct {
	CreatedAt string              `json:"created_at,omitempty"`
	UpdatedAt string              `json:"updated_at,omitempty"`
	Filters   study.RunFilters    `json:"filters"`
	Config    study.ConfigSummary `json:"config_summary"`
}

type statusTaskJSON struct {
	ID          string                   `json:"id"`
	Kind        study.TaskKind           `json:"kind"`
	Status      study.TaskStatus         `json:"status"`
	Dimension   string                   `json:"dimension,omitempty"`
	Source      string                   `json:"source,omitempty"`
	OutputPath  string                   `json:"output_path"`
	Attempts    int                      `json:"attempts"`
	RetryAfter  string                   `json:"retry_after,omitempty"`
	LastError   *study.TaskError         `json:"last_error,omitempty"`
	Validation  *study.ValidationSummary `json:"validation,omitempty"`
	AgentStatus string                   `json:"agent_status,omitempty"`
	Usage       knownValue[int64]        `json:"usage"`
	Cost        knownValue[float64]      `json:"cost"`
}

type knownValue[T any] struct {
	Known bool `json:"known"`
	Value *T   `json:"value"`
}

func statusJSONResult(root string, state study.RunState, summary study.StatusSummary) statusJSON {
	out := statusJSON{
		SchemaVersion: 1,
		RunID:         summary.RunID,
		Complete:      summary.Complete,
		StatePath:     workspace.Rel(root, summary.StatePath),
		Counts: statusCounts{
			Total: summary.Total, Pending: summary.Pending, Running: summary.Running,
			Validating: summary.Validating, Completed: summary.Completed, Failed: summary.Failed,
			Cancelled: summary.Cancelled, Skipped: summary.Skipped, Waiting: summary.Waiting,
			Retrying: summary.Retrying, Active: summary.Active, Retries: summary.RetryCount,
		},
		RunMetadata: statusRunMetadata{
			CreatedAt: state.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt: state.UpdatedAt.UTC().Format(time.RFC3339),
			Filters:   state.Filters,
			Config:    state.Config,
		},
		Usage: knownValue[int64]{Known: false},
		Cost:  knownValue[float64]{Known: false},
	}
	if summary.Lock != nil {
		lock := *summary.Lock
		lock.Path = workspace.Rel(root, lock.Path)
		lock.Command = config.RedactValue("lock.command", lock.Command)
		out.Lock = &lock
	}
	var totalTokens int64
	var totalTokensKnown bool
	var totalCost float64
	var totalCostKnown bool
	for _, task := range summary.Tasks {
		item := statusTaskJSON{
			ID: task.ID, Kind: task.Kind, Status: task.Status, Dimension: task.DimensionRef,
			Source: task.Source, OutputPath: workspace.Rel(root, task.OutputPath), Attempts: task.Attempts,
			AgentStatus: task.Agent.Status,
			Usage:       knownValue[int64]{Known: false},
			Cost:        knownValue[float64]{Known: false},
		}
		if item.Dimension == "" {
			item.Dimension = task.Dimension
		}
		if task.RetryAfter != nil {
			item.RetryAfter = task.RetryAfter.UTC().Format(time.RFC3339)
		}
		if task.LastError != nil {
			errCopy := *task.LastError
			errCopy.Message = config.RedactValue("task.error", errCopy.Message)
			errCopy.Path = workspace.Rel(root, errCopy.Path)
			item.LastError = &errCopy
		}
		if task.Validation != nil {
			validation := *task.Validation
			validation.Path = workspace.Rel(root, validation.Path)
			validation.Message = config.RedactValue("validation.message", validation.Message)
			item.Validation = &validation
		}
		if task.Agent.Usage.TotalTokensKnown {
			value := task.Agent.Usage.TotalTokens
			item.Usage = knownValue[int64]{Known: true, Value: &value}
			totalTokens += value
			totalTokensKnown = true
		}
		if task.Agent.Cost != nil {
			value := task.Agent.Cost.Amount
			item.Cost = knownValue[float64]{Known: true, Value: &value}
			totalCost += value
			totalCostKnown = true
		}
		out.Tasks = append(out.Tasks, item)
	}
	if totalTokensKnown {
		out.Usage = knownValue[int64]{Known: true, Value: &totalTokens}
	}
	if totalCostKnown {
		out.Cost = knownValue[float64]{Known: true, Value: &totalCost}
	}
	return out
}
