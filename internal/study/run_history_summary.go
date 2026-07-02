package study

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func WriteRunHistorySummary(study Study, state RunState) error {
	records, err := LoadRunHistory(study)
	if err != nil {
		return err
	}
	content := RenderRunHistorySummary(study, state, records, time.Now().UTC())
	path := RunHistorySummaryPath(study)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func RenderRunHistorySummary(study Study, state RunState, records []RunHistoryRecord, now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Study Run Summary\n\n")
	fmt.Fprintf(&b, "- Study: `%s`\n", study.Name)
	fmt.Fprintf(&b, "- Updated: `%s`\n", now.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Run state: `%s`\n", filepath.Join(RunStateDirName, RunStateFileName))
	fmt.Fprintf(&b, "- Ledger: `%s`\n\n", filepath.Join(RunStateDirName, RunHistoryDirName, RunHistoryFileName))

	counts := historyCounts(records)
	pending := remainingTasks(state)
	fmt.Fprintf(&b, "## Status\n\n")
	fmt.Fprintf(&b, "| Metric | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| Runs recorded | %d |\n", len(records))
	fmt.Fprintf(&b, "| Completed | %d |\n", counts.completed)
	fmt.Fprintf(&b, "| Failed | %d |\n", counts.failed)
	fmt.Fprintf(&b, "| Cancelled | %d |\n", counts.cancelled)
	fmt.Fprintf(&b, "| Skipped | %d |\n", counts.skipped)
	fmt.Fprintf(&b, "| Remaining tasks | %d |\n", len(pending))
	fmt.Fprintf(&b, "| Dimensions seen | %d |\n", len(counts.dimensions))
	fmt.Fprintf(&b, "| Sources seen | %d |\n\n", len(counts.sources))

	writeRemaining(&b, pending)
	writeDimensionSummary(&b, records)
	writeSourceSummary(&b, records)
	writeRuntimeSummary(&b, records)
	writeRecentRuns(&b, records)
	writeSlowestRuns(&b, records)
	writeFailedRuns(&b, records)
	return b.String()
}

type historyCounter struct {
	completed  int
	failed     int
	cancelled  int
	skipped    int
	dimensions map[string]bool
	sources    map[string]bool
}

func historyCounts(records []RunHistoryRecord) historyCounter {
	out := historyCounter{dimensions: map[string]bool{}, sources: map[string]bool{}}
	for _, record := range records {
		switch record.Status {
		case TaskStatusCompleted:
			out.completed++
		case TaskStatusFailed:
			out.failed++
		case TaskStatusCancelled:
			out.cancelled++
		case TaskStatusSkipped:
			out.skipped++
		}
		if record.DimensionRef != "" {
			out.dimensions[record.DimensionRef] = true
		}
		if record.Source != "" {
			out.sources[record.Source] = true
		}
	}
	return out
}

func remainingTasks(state RunState) []TaskState {
	var out []TaskState
	for _, task := range state.Tasks {
		if !terminalTaskStatus(task.Status) {
			out = append(out, task)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func writeRemaining(b *strings.Builder, tasks []TaskState) {
	fmt.Fprintf(b, "## Remaining Work\n\n")
	if len(tasks) == 0 {
		fmt.Fprintf(b, "No remaining tasks in the current run state.\n\n")
		return
	}
	fmt.Fprintf(b, "| Dimension | Source | Kind | Status |\n| --- | --- | --- | --- |\n")
	for _, task := range tasks {
		fmt.Fprintf(b, "| %s | %s | %s | %s |\n", md(task.DimensionRef), md(displaySource(task)), task.Kind, task.Status)
	}
	fmt.Fprintf(b, "\n")
}

func writeDimensionSummary(b *strings.Builder, records []RunHistoryRecord) {
	type agg struct {
		rows []RunHistoryRecord
	}
	items := map[string]*agg{}
	for _, record := range records {
		key := record.DimensionRef
		if key == "" {
			key = "(none)"
		}
		if items[key] == nil {
			items[key] = &agg{}
		}
		items[key].rows = append(items[key].rows, record)
	}
	keys := sortedKeys(items)
	fmt.Fprintf(b, "## Dimensions\n\n")
	if len(keys) == 0 {
		fmt.Fprintf(b, "No runs recorded yet.\n\n")
		return
	}
	fmt.Fprintf(b, "| Dimension | Runs | Completed | Failed | Avg Duration | Tokens | Cost |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, key := range keys {
		a := summarizeRecords(items[key].rows)
		fmt.Fprintf(b, "| %s | %d | %d | %d | %s | %s | %s |\n", md(key), a.runs, a.completed, a.failed, formatDuration(a.avgDuration), formatIntKnown(a.totalTokens, a.totalTokensKnown), formatCost(a.costAmount, a.costCurrency, a.costKnown))
	}
	fmt.Fprintf(b, "\n")
}

func writeSourceSummary(b *strings.Builder, records []RunHistoryRecord) {
	items := map[string][]RunHistoryRecord{}
	for _, record := range records {
		if record.Kind != TaskKindAnalysis {
			continue
		}
		items[record.Source] = append(items[record.Source], record)
	}
	keys := sortedRecordKeys(items)
	fmt.Fprintf(b, "## Sources\n\n")
	if len(keys) == 0 {
		fmt.Fprintf(b, "No source runs recorded yet.\n\n")
		return
	}
	fmt.Fprintf(b, "| Source | Runs | Completed | Failed | Avg Duration | Tokens | Cost |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, key := range keys {
		a := summarizeRecords(items[key])
		fmt.Fprintf(b, "| %s | %d | %d | %d | %s | %s | %s |\n", md(key), a.runs, a.completed, a.failed, formatDuration(a.avgDuration), formatIntKnown(a.totalTokens, a.totalTokensKnown), formatCost(a.costAmount, a.costCurrency, a.costKnown))
	}
	fmt.Fprintf(b, "\n")
}

func writeRuntimeSummary(b *strings.Builder, records []RunHistoryRecord) {
	items := map[string][]RunHistoryRecord{}
	for _, record := range records {
		key := strings.Join([]string{emptyDash(record.Runtime), emptyDash(record.Provider), emptyDash(record.Model)}, " / ")
		items[key] = append(items[key], record)
	}
	keys := sortedRecordKeys(items)
	fmt.Fprintf(b, "## Runtime And Model\n\n")
	if len(keys) == 0 {
		fmt.Fprintf(b, "No runtime metadata recorded yet.\n\n")
		return
	}
	fmt.Fprintf(b, "| Runtime / Provider / Model | Runs | Completed | Failed | Avg Duration | Tokens | Cost |\n| --- | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, key := range keys {
		a := summarizeRecords(items[key])
		fmt.Fprintf(b, "| %s | %d | %d | %d | %s | %s | %s |\n", md(key), a.runs, a.completed, a.failed, formatDuration(a.avgDuration), formatIntKnown(a.totalTokens, a.totalTokensKnown), formatCost(a.costAmount, a.costCurrency, a.costKnown))
	}
	fmt.Fprintf(b, "\n")
}

func writeRecentRuns(b *strings.Builder, records []RunHistoryRecord) {
	rows := append([]RunHistoryRecord(nil), records...)
	sort.Slice(rows, func(i, j int) bool { return timeAfter(rows[i].CompletedAt, rows[j].CompletedAt) })
	if len(rows) > 20 {
		rows = rows[:20]
	}
	fmt.Fprintf(b, "## Recent Runs\n\n")
	writeRunTable(b, rows, "No recent runs recorded yet.")
}

func writeSlowestRuns(b *strings.Builder, records []RunHistoryRecord) {
	rows := append([]RunHistoryRecord(nil), records...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].DurationMS > rows[j].DurationMS })
	if len(rows) > 10 {
		rows = rows[:10]
	}
	fmt.Fprintf(b, "## Slowest Runs\n\n")
	writeRunTable(b, rows, "No duration data recorded yet.")
}

func writeFailedRuns(b *strings.Builder, records []RunHistoryRecord) {
	var rows []RunHistoryRecord
	for _, record := range records {
		if record.Status == TaskStatusFailed || record.Status == TaskStatusCancelled {
			rows = append(rows, record)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return timeAfter(rows[i].CompletedAt, rows[j].CompletedAt) })
	fmt.Fprintf(b, "## Failed Or Cancelled Runs\n\n")
	if len(rows) == 0 {
		fmt.Fprintf(b, "No failed or cancelled runs recorded.\n\n")
		return
	}
	fmt.Fprintf(b, "| Completed | Dimension | Source | Status | Error |\n| --- | --- | --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n", formatTime(row.CompletedAt), md(row.DimensionRef), md(displayRecordSource(row)), row.Status, md(firstNonEmpty(row.ErrorMessage, row.ErrorCode, "-")))
	}
	fmt.Fprintf(b, "\n")
}

func writeRunTable(b *strings.Builder, rows []RunHistoryRecord, empty string) {
	if len(rows) == 0 {
		fmt.Fprintf(b, "%s\n\n", empty)
		return
	}
	fmt.Fprintf(b, "| Completed | Dimension | Source | Kind | Status | Duration | Model | Tokens | Cost |\n| --- | --- | --- | --- | --- | ---: | --- | ---: | ---: |\n")
	for _, row := range rows {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s |\n", formatTime(row.CompletedAt), md(row.DimensionRef), md(displayRecordSource(row)), row.Kind, row.Status, formatDuration(time.Duration(row.DurationMS)*time.Millisecond), md(emptyDash(row.Model)), formatIntKnown(row.TotalTokens, row.TotalTokensKnown), formatCost(row.CostAmount, row.CostCurrency, row.CostKnown))
	}
	fmt.Fprintf(b, "\n")
}

type recordSummary struct {
	runs             int
	completed        int
	failed           int
	totalDuration    time.Duration
	avgDuration      time.Duration
	totalTokens      int64
	totalTokensKnown bool
	costAmount       float64
	costCurrency     string
	costKnown        bool
}

func summarizeRecords(records []RunHistoryRecord) recordSummary {
	var out recordSummary
	for _, record := range records {
		out.runs++
		if record.Status == TaskStatusCompleted {
			out.completed++
		}
		if record.Status == TaskStatusFailed {
			out.failed++
		}
		out.totalDuration += time.Duration(record.DurationMS) * time.Millisecond
		if record.TotalTokensKnown {
			out.totalTokensKnown = true
			out.totalTokens += record.TotalTokens
		}
		if record.CostKnown {
			out.costKnown = true
			out.costAmount += record.CostAmount
			if out.costCurrency == "" {
				out.costCurrency = record.CostCurrency
			}
			if out.costCurrency != record.CostCurrency {
				out.costCurrency = "mixed"
			}
		}
	}
	if out.runs > 0 {
		out.avgDuration = out.totalDuration / time.Duration(out.runs)
	}
	return out
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedRecordKeys(m map[string][]RunHistoryRecord) []string {
	return sortedKeys(m)
}

func displaySource(task TaskState) string {
	if task.Source == "" {
		return "(synthesis)"
	}
	return task.Source
}

func displayRecordSource(record RunHistoryRecord) string {
	if record.Source == "" {
		return "(synthesis)"
	}
	return record.Source
}

func formatDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
}

func formatIntKnown(v int64, known bool) string {
	if !known {
		return "-"
	}
	return fmt.Sprintf("%d", v)
}

func formatCost(amount float64, currency string, known bool) string {
	if !known {
		return "-"
	}
	if currency == "" {
		currency = "cost"
	}
	return fmt.Sprintf("%.4f %s", amount, currency)
}

func formatTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format(time.RFC3339)
}

func timeAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func md(s string) string {
	if s == "" {
		return "-"
	}
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
