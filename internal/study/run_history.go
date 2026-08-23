package study

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

const (
	RunHistorySchemaVersion = 1
	RunHistoryDirName       = "runs"
	RunHistoryFileName      = "tasks.jsonl"
	RunHistorySummaryName   = "summary.md"
)

type RunHistoryRecord struct {
	SchemaVersion int        `json:"schema_version"`
	Key           string     `json:"key"`
	Study         string     `json:"study"`
	RunID         string     `json:"run_id"`
	TaskID        string     `json:"task_id"`
	Kind          TaskKind   `json:"kind"`
	Status        TaskStatus `json:"status"`

	DimensionRef string     `json:"dimension_ref,omitempty"`
	Dimension    string     `json:"dimension,omitempty"`
	Source       string     `json:"source,omitempty"`
	SourceKind   SourceKind `json:"source_kind,omitempty"`
	OutputPath   string     `json:"output_path,omitempty"`

	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DurationMS  int64      `json:"duration_ms,omitempty"`
	Attempts    int        `json:"attempts,omitempty"`

	Runtime    string `json:"runtime,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	AgentRunID string `json:"agent_run_id,omitempty"`

	InputTokensKnown  bool  `json:"input_tokens_known"`
	InputTokens       int64 `json:"input_tokens,omitempty"`
	OutputTokensKnown bool  `json:"output_tokens_known"`
	OutputTokens      int64 `json:"output_tokens,omitempty"`
	TotalTokensKnown  bool  `json:"total_tokens_known"`
	TotalTokens       int64 `json:"total_tokens,omitempty"`

	CostKnown    bool    `json:"cost_known"`
	CostAmount   float64 `json:"cost_amount,omitempty"`
	CostCurrency string  `json:"cost_currency,omitempty"`
	CostEstimate bool    `json:"cost_estimate,omitempty"`
	CostSource   string  `json:"cost_source,omitempty"`

	ValidationStatus       ValidationStatus `json:"validation_status,omitempty"`
	ValidationPassedChecks int              `json:"validation_passed_checks,omitempty"`
	ValidationFailedChecks int              `json:"validation_failed_checks,omitempty"`
	ErrorCode              string           `json:"error_code,omitempty"`
	ErrorMessage           string           `json:"error_message,omitempty"`
	RecordedAt             time.Time        `json:"recorded_at"`
}

func RunHistoryPath(study Study) string {
	return filepath.Join(study.Path, RunStateDirName, RunHistoryDirName, RunHistoryFileName)
}

func RunHistorySummaryPath(study Study) string {
	return filepath.Join(study.Path, RunStateDirName, RunHistoryDirName, RunHistorySummaryName)
}

func AppendRunHistory(study Study, state RunState, task TaskState) error {
	existing, err := readRunHistoryKeys(RunHistoryPath(study))
	if err != nil {
		return err
	}
	return appendRunHistoryWithKeys(study, state, task, existing)
}

func appendRunHistoryWithKeys(study Study, state RunState, task TaskState, existing map[string]bool) error {
	if !terminalTaskStatus(task.Status) || task.CompletedAt == nil {
		return nil
	}
	record := NewRunHistoryRecord(study, state, task, time.Now().UTC())
	if existing[record.Key] {
		return nil
	}
	path := RunHistoryPath(study)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	previous, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	previous = trimInvalidTrailingRunHistory(previous)
	if len(previous) > 0 && previous[len(previous)-1] != '\n' {
		previous = append(previous, '\n')
	}
	contents := append(previous, append(data, '\n')...)
	if err := atomicWriteRunHistory(path, contents); err != nil {
		return err
	}
	existing[record.Key] = true
	return nil
}

func trimInvalidTrailingRunHistory(data []byte) []byte {
	trimmed := bytes.TrimRight(data, "\r\n")
	if len(trimmed) == 0 {
		return nil
	}
	lineStart := bytes.LastIndexByte(trimmed, '\n') + 1
	if json.Valid(bytes.TrimSpace(trimmed[lineStart:])) {
		return data
	}
	return append([]byte(nil), trimmed[:lineStart]...)
}

func atomicWriteRunHistory(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tasks.*.jsonl.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func SyncRunHistory(study Study, state RunState) error {
	existing, err := readRunHistoryKeys(RunHistoryPath(study))
	if err != nil {
		return err
	}
	for _, task := range state.Tasks {
		if err := appendRunHistoryWithKeys(study, state, task, existing); err != nil {
			return err
		}
	}
	return WriteRunHistorySummary(study, state)
}

func NewRunHistoryRecord(study Study, state RunState, task TaskState, recordedAt time.Time) RunHistoryRecord {
	if recordedAt.IsZero() {
		recordedAt = time.Now().UTC()
	}
	record := RunHistoryRecord{
		SchemaVersion: RunHistorySchemaVersion,
		Study:         study.Name,
		RunID:         state.RunID,
		TaskID:        task.ID,
		Kind:          task.Kind,
		Status:        task.Status,
		DimensionRef:  task.DimensionRef,
		Dimension:     task.Dimension,
		Source:        task.Source,
		SourceKind:    task.SourceKind,
		OutputPath:    task.OutputPath,
		StartedAt:     cloneTimePtr(task.StartedAt),
		CompletedAt:   cloneTimePtr(task.CompletedAt),
		Attempts:      task.Attempts,
		Runtime:       task.Agent.Runtime,
		Provider:      task.Agent.Provider,
		Model:         task.Agent.Model,
		AgentRunID:    task.Agent.RunID,
		RecordedAt:    recordedAt,
	}
	if record.Runtime == "" {
		record.Runtime = state.Config.Runtime
	}
	if record.Model == "" {
		record.Model = state.Config.Model
	}
	record.InputTokensKnown = task.Agent.Usage.InputTokensKnown
	record.InputTokens = task.Agent.Usage.InputTokens
	record.OutputTokensKnown = task.Agent.Usage.OutputTokensKnown
	record.OutputTokens = task.Agent.Usage.OutputTokens
	record.TotalTokensKnown = task.Agent.Usage.TotalTokensKnown
	record.TotalTokens = task.Agent.Usage.TotalTokens
	if task.Agent.Cost != nil {
		record.CostKnown = true
		record.CostAmount = task.Agent.Cost.Amount
		record.CostCurrency = task.Agent.Cost.Currency
		record.CostEstimate = task.Agent.Cost.Estimate
		record.CostSource = task.Agent.Cost.Source
	}
	if task.Validation != nil {
		record.ValidationStatus = task.Validation.Status
		record.ValidationPassedChecks = task.Validation.PassedChecks
		record.ValidationFailedChecks = task.Validation.FailedChecks
	}
	if task.LastError != nil {
		record.ErrorCode = task.LastError.Code
		record.ErrorMessage = task.LastError.Message
	}
	if task.StartedAt != nil && task.CompletedAt != nil {
		record.DurationMS = task.CompletedAt.Sub(*task.StartedAt).Milliseconds()
		if record.DurationMS < 0 {
			record.DurationMS = 0
		}
	}
	record.Key = runHistoryKey(record)
	return record
}

func LoadRunHistory(study Study) ([]RunHistoryRecord, error) {
	path := RunHistoryPath(study)
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	return readRunHistory(file)
}

func readRunHistory(r io.Reader) ([]RunHistoryRecord, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var records []RunHistoryRecord
	var pending []byte
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		if len(pending) > 0 {
			record, err := decodeRunHistoryRecord(pending)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
		if len(line) == 0 {
			pending = nil
			continue
		}
		pending = line
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(pending) > 0 {
		record, err := decodeRunHistoryRecord(pending)
		if err == nil {
			records = append(records, record)
		}
		// A crash or full disk can leave only the final JSONL record partial.
		// Earlier malformed records still fail above because ignoring them would
		// hide ledger corruption rather than recover an interrupted append.
	}
	return records, nil
}

func decodeRunHistoryRecord(line []byte) (RunHistoryRecord, error) {
	var record RunHistoryRecord
	if err := json.Unmarshal(line, &record); err != nil {
		return RunHistoryRecord{}, err
	}
	return record, nil
}

func readRunHistoryKeys(path string) (map[string]bool, error) {
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	defer file.Close()
	records, err := readRunHistory(file)
	if err != nil {
		return nil, err
	}
	keys := make(map[string]bool, len(records))
	for _, record := range records {
		keys[record.Key] = true
	}
	return keys, nil
}

func terminalTaskStatus(status TaskStatus) bool {
	return status == TaskStatusCompleted || status == TaskStatusFailed || status == TaskStatusCancelled || status == TaskStatusSkipped
}

func runHistoryKey(record RunHistoryRecord) string {
	completed := ""
	if record.CompletedAt != nil {
		completed = record.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	return fmt.Sprintf("%s|%s|%d|%s", record.RunID, record.TaskID, record.Attempts, completed)
}

func cloneTimePtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	out := *t
	return &out
}
