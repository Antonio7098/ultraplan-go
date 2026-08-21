package study

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	runLoopDiagnosticsInterval = 5 * time.Second
	runLoopDiagnosticsMaxBytes = 8 * 1024 * 1024
)

type runLoopDiagnostics struct {
	path  string
	runID string
	mu    sync.Mutex
}

type runLoopMemorySample struct {
	Timestamp            time.Time `json:"timestamp"`
	RunID                string    `json:"run_id,omitempty"`
	Phase                string    `json:"phase"`
	TaskID               string    `json:"task_id,omitempty"`
	DurationMS           int64     `json:"duration_ms,omitempty"`
	StateBytes           int64     `json:"state_bytes,omitempty"`
	HeapAllocBytes       uint64    `json:"heap_alloc_bytes"`
	HeapInuseBytes       uint64    `json:"heap_inuse_bytes"`
	HeapSysBytes         uint64    `json:"heap_sys_bytes"`
	ProcessRSSBytes      uint64    `json:"process_rss_bytes,omitempty"`
	ProcessHWMBytes      uint64    `json:"process_hwm_bytes,omitempty"`
	ProcessSwap          uint64    `json:"process_swap_bytes,omitempty"`
	Goroutines           int       `json:"goroutines"`
	NumGC                uint32    `json:"num_gc"`
	Error                string    `json:"error,omitempty"`
	RequestedParallelism int       `json:"requested_parallelism,omitempty"`
	EffectiveParallelism int       `json:"effective_parallelism,omitempty"`
	MemoryAvailableBytes uint64    `json:"memory_available_bytes,omitempty"`
	ChildProcessCount    int       `json:"child_process_count,omitempty"`
	ChildRSSBytes        uint64    `json:"child_rss_bytes,omitempty"`
}

func newRunLoopDiagnostics(study Study, runID string) *runLoopDiagnostics {
	return &runLoopDiagnostics{
		path:  filepath.Join(study.Path, RunStateDirName, "diagnostics", "run-loop-memory.jsonl"),
		runID: runID,
	}
}

func (d *runLoopDiagnostics) start(ctx context.Context) func() {
	ctx, cancel := context.WithCancel(ctx)
	d.sample("run_loop.start", "", 0, nil)
	go func() {
		ticker := time.NewTicker(runLoopDiagnosticsInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				d.sample("run_loop.periodic", "", 0, nil)
			}
		}
	}()
	return func() {
		cancel()
		d.sample("run_loop.stop", "", 0, nil)
	}
}

func (d *runLoopDiagnostics) sample(phase, taskID string, duration time.Duration, sampleErr error) {
	if d == nil {
		return
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	rss, hwm, swap := processMemory()
	childCount, childRSS := childProcessMemory()
	sample := runLoopMemorySample{
		Timestamp:         time.Now().UTC(),
		RunID:             d.runID,
		Phase:             phase,
		TaskID:            taskID,
		DurationMS:        duration.Milliseconds(),
		StateBytes:        fileSize(filepath.Join(filepath.Dir(filepath.Dir(d.path)), RunStateFileName)),
		HeapAllocBytes:    mem.HeapAlloc,
		HeapInuseBytes:    mem.HeapInuse,
		HeapSysBytes:      mem.HeapSys,
		ProcessRSSBytes:   rss,
		ProcessHWMBytes:   hwm,
		ProcessSwap:       swap,
		Goroutines:        runtime.NumGoroutine(),
		NumGC:             mem.NumGC,
		ChildProcessCount: childCount,
		ChildRSSBytes:     childRSS,
	}
	if sampleErr != nil {
		sample.Error = compactDiagnostic(sampleErr.Error())
	}
	d.append(sample)
}

func (d *runLoopDiagnostics) scheduling(phase string, requested, effective int, available uint64) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	rss, hwm, swap := processMemory()
	childCount, childRSS := childProcessMemory()
	d.append(runLoopMemorySample{Timestamp: time.Now().UTC(), RunID: d.runID, Phase: phase,
		StateBytes:     fileSize(filepath.Join(filepath.Dir(filepath.Dir(d.path)), RunStateFileName)),
		HeapAllocBytes: mem.HeapAlloc, HeapInuseBytes: mem.HeapInuse, HeapSysBytes: mem.HeapSys,
		ProcessRSSBytes: rss, ProcessHWMBytes: hwm, ProcessSwap: swap, Goroutines: runtime.NumGoroutine(), NumGC: mem.NumGC,
		RequestedParallelism: requested, EffectiveParallelism: effective, MemoryAvailableBytes: available,
		ChildProcessCount: childCount, ChildRSSBytes: childRSS})
}

func (d *runLoopDiagnostics) append(sample runLoopMemorySample) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(d.path), 0o755); err != nil {
		return
	}
	if info, err := os.Stat(d.path); err == nil && info.Size() >= runLoopDiagnosticsMaxBytes {
		_ = os.Remove(d.path + ".1")
		_ = os.Rename(d.path, d.path+".1")
	}
	file, err := os.OpenFile(d.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_ = json.NewEncoder(file).Encode(sample)
	_ = file.Close()
}

func processMemory() (rss, hwm, swap uint64) {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, 0, 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "VmRSS":
			rss = value * 1024
		case "VmHWM":
			hwm = value * 1024
		case "VmSwap":
			swap = value * 1024
		}
	}
	return rss, hwm, swap
}

func childProcessMemory() (count int, rss uint64) {
	data, err := os.ReadFile("/proc/self/task/" + strconv.Itoa(os.Getpid()) + "/children")
	if err != nil {
		return 0, 0
	}
	for _, value := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		childRSS, _, _ := processMemoryForPID(pid)
		count++
		rss += childRSS
	}
	return count, rss
}

func processMemoryForPID(pid int) (rss, hwm, swap uint64) {
	file, err := os.Open("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return 0, 0, 0
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch strings.TrimSuffix(fields[0], ":") {
		case "VmRSS":
			rss = value * 1024
		case "VmHWM":
			hwm = value * 1024
		case "VmSwap":
			swap = value * 1024
		}
	}
	return rss, hwm, swap
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
