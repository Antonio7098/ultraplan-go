package study

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	runtimepkg "github.com/Antonio7098/ultraplan-go/internal/platform/runtime"
)

const (
	runLoopDiagnosticsInterval = 5 * time.Second
	runLoopDiagnosticsMaxBytes = 8 * 1024 * 1024
)

type runLoopDiagnostics struct {
	path                 string
	studyRoot            string
	runID                string
	mu                   sync.Mutex
	activeTasks          map[string]struct{}
	requestedParallelism int
	effectiveParallelism int
}

type RunLoopChildResource struct {
	PID       int    `json:"pid"`
	TaskID    string `json:"task_id,omitempty"`
	RSSBytes  uint64 `json:"rss_bytes,omitempty"`
	SwapBytes uint64 `json:"swap_bytes,omitempty"`
	CPUTimeMS uint64 `json:"cpu_time_ms,omitempty"`
	ElapsedMS uint64 `json:"elapsed_ms,omitempty"`
}

type RunLoopMemorySample struct {
	Timestamp            time.Time              `json:"timestamp"`
	RunID                string                 `json:"run_id,omitempty"`
	Phase                string                 `json:"phase"`
	TaskID               string                 `json:"task_id,omitempty"`
	DurationMS           int64                  `json:"duration_ms,omitempty"`
	StateBytes           int64                  `json:"state_bytes,omitempty"`
	HeapAllocBytes       uint64                 `json:"heap_alloc_bytes"`
	HeapInuseBytes       uint64                 `json:"heap_inuse_bytes"`
	HeapSysBytes         uint64                 `json:"heap_sys_bytes"`
	ProcessRSSBytes      uint64                 `json:"process_rss_bytes,omitempty"`
	ProcessHWMBytes      uint64                 `json:"process_hwm_bytes,omitempty"`
	ProcessSwap          uint64                 `json:"process_swap_bytes,omitempty"`
	Goroutines           int                    `json:"goroutines"`
	NumGC                uint32                 `json:"num_gc"`
	Error                string                 `json:"error,omitempty"`
	RequestedParallelism int                    `json:"requested_parallelism,omitempty"`
	EffectiveParallelism int                    `json:"effective_parallelism,omitempty"`
	MemoryAvailableBytes uint64                 `json:"memory_available_bytes,omitempty"`
	ChildProcessCount    int                    `json:"child_process_count,omitempty"`
	ChildRSSBytes        uint64                 `json:"child_rss_bytes,omitempty"`
	ActiveTaskIDs        []string               `json:"active_task_ids,omitempty"`
	Children             []RunLoopChildResource `json:"children,omitempty"`
	DiskTotalBytes       uint64                 `json:"disk_total_bytes,omitempty"`
	DiskAvailableBytes   uint64                 `json:"disk_available_bytes,omitempty"`
	DiskUsedPercent      float64                `json:"disk_used_percent,omitempty"`
	AdmissionPaused      bool                   `json:"admission_paused,omitempty"`
	RuntimeStoreBytes    uint64                 `json:"runtime_store_bytes,omitempty"`
	RuntimeStoreCount    int                    `json:"runtime_store_count,omitempty"`
	RuntimeStores        []RunLoopStoreResource `json:"runtime_stores,omitempty"`
	StoresRemoved        int                    `json:"stores_removed,omitempty"`
	StoreCleanupErrors   []string               `json:"store_cleanup_errors,omitempty"`
}

type RunLoopStoreResource struct {
	Owner     string                       `json:"owner"`
	State     runtimepkg.RuntimeStoreState `json:"state"`
	Bytes     uint64                       `json:"bytes"`
	UpdatedAt time.Time                    `json:"updated_at"`
}

type RunLoopResourceHistory struct {
	Study   string                `json:"study"`
	Samples []RunLoopMemorySample `json:"samples"`
}

type runLoopMemorySample = RunLoopMemorySample

func newRunLoopDiagnostics(study Study, runID string) *runLoopDiagnostics {
	return &runLoopDiagnostics{
		path: filepath.Join(study.Path, RunStateDirName, "diagnostics", "run-loop-memory.jsonl"), studyRoot: study.Path, runID: runID,
		activeTasks: map[string]struct{}{},
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

func (d *runLoopDiagnostics) configureParallelism(requested int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.requestedParallelism = requested
	d.effectiveParallelism = requested
}

func (d *runLoopDiagnostics) sample(phase, taskID string, duration time.Duration, sampleErr error) {
	if d == nil {
		return
	}
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	rss, hwm, swap := processMemory()
	d.mu.Lock()
	if phase == "runtime.start" && taskID != "" {
		d.activeTasks[taskID] = struct{}{}
	}
	activeTasks := sortedTaskIDs(d.activeTasks)
	requestedParallelism := d.requestedParallelism
	effectiveParallelism := d.effectiveParallelism
	children := childProcessResources(activeTasks)
	disk := readDiskPressure(d.studyRoot)
	stores, storeBytes := runtimeStoreResources(d.studyRoot)
	if phase == "runtime.end" && taskID != "" {
		delete(d.activeTasks, taskID)
	}
	d.mu.Unlock()
	childRSS := totalChildRSS(children)
	memoryAvailable := readMemoryPressure().AvailableBytes
	sample := RunLoopMemorySample{
		Timestamp:            time.Now().UTC(),
		RunID:                d.runID,
		Phase:                phase,
		TaskID:               taskID,
		DurationMS:           duration.Milliseconds(),
		StateBytes:           fileSize(filepath.Join(filepath.Dir(filepath.Dir(d.path)), RunStateFileName)),
		HeapAllocBytes:       mem.HeapAlloc,
		HeapInuseBytes:       mem.HeapInuse,
		HeapSysBytes:         mem.HeapSys,
		ProcessRSSBytes:      rss,
		ProcessHWMBytes:      hwm,
		ProcessSwap:          swap,
		Goroutines:           runtime.NumGoroutine(),
		NumGC:                mem.NumGC,
		ChildProcessCount:    len(children),
		ChildRSSBytes:        childRSS,
		ActiveTaskIDs:        activeTasks,
		Children:             children,
		DiskTotalBytes:       disk.TotalBytes,
		DiskAvailableBytes:   disk.AvailableBytes,
		DiskUsedPercent:      disk.UsedPercent,
		AdmissionPaused:      disk.Pressured,
		RuntimeStoreBytes:    storeBytes,
		RuntimeStoreCount:    len(stores),
		RuntimeStores:        stores,
		RequestedParallelism: requestedParallelism,
		EffectiveParallelism: effectiveParallelism,
		MemoryAvailableBytes: memoryAvailable,
	}
	if sampleErr != nil {
		sample.Error = compactDiagnostic(sampleErr.Error())
	}
	d.append(sample)
}

func (d *runLoopDiagnostics) storage(phase string, cleanup runtimepkg.RuntimeStoreCleanup, disk diskPressure) {
	if d == nil {
		return
	}
	stores, storeBytes := runtimeStoreResources(d.studyRoot)
	d.append(RunLoopMemorySample{Timestamp: time.Now().UTC(), RunID: d.runID, Phase: phase,
		DiskTotalBytes: disk.TotalBytes, DiskAvailableBytes: disk.AvailableBytes, DiskUsedPercent: disk.UsedPercent,
		AdmissionPaused: disk.Pressured, RuntimeStoreBytes: storeBytes, RuntimeStoreCount: len(stores), RuntimeStores: stores,
		StoresRemoved: len(cleanup.Removed), StoreCleanupErrors: cleanup.Failed})
}

func runtimeStoreResources(studyRoot string) ([]RunLoopStoreResource, uint64) {
	stores, err := runtimepkg.InspectRuntimeStores(studyRoot)
	if err != nil {
		return nil, 0
	}
	resources := make([]RunLoopStoreResource, 0, len(stores))
	var total uint64
	for _, store := range stores {
		total += store.Bytes
		resources = append(resources, RunLoopStoreResource{Owner: store.Owner, State: store.State, Bytes: store.Bytes, UpdatedAt: store.UpdatedAt})
	}
	return resources, total
}

func (d *runLoopDiagnostics) scheduling(phase string, requested, effective int, available uint64) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	rss, hwm, swap := processMemory()
	d.mu.Lock()
	d.requestedParallelism = requested
	d.effectiveParallelism = effective
	activeTasks := sortedTaskIDs(d.activeTasks)
	d.mu.Unlock()
	children := childProcessResources(activeTasks)
	d.append(RunLoopMemorySample{Timestamp: time.Now().UTC(), RunID: d.runID, Phase: phase,
		StateBytes:     fileSize(filepath.Join(filepath.Dir(filepath.Dir(d.path)), RunStateFileName)),
		HeapAllocBytes: mem.HeapAlloc, HeapInuseBytes: mem.HeapInuse, HeapSysBytes: mem.HeapSys,
		ProcessRSSBytes: rss, ProcessHWMBytes: hwm, ProcessSwap: swap, Goroutines: runtime.NumGoroutine(), NumGC: mem.NumGC,
		RequestedParallelism: requested, EffectiveParallelism: effective, MemoryAvailableBytes: available,
		ChildProcessCount: len(children), ChildRSSBytes: totalChildRSS(children), ActiveTaskIDs: activeTasks, Children: children})
}

func (d *runLoopDiagnostics) append(sample RunLoopMemorySample) {
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

func childProcessResources(activeTasks []string) []RunLoopChildResource {
	data, err := os.ReadFile("/proc/self/task/" + strconv.Itoa(os.Getpid()) + "/children")
	if err != nil {
		return nil
	}
	var children []RunLoopChildResource
	for _, value := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(value)
		if err != nil {
			continue
		}
		childRSS, _, childSwap := processMemoryForPID(pid)
		cpuMS, elapsedMS := processTimes(pid)
		children = append(children, RunLoopChildResource{PID: pid, TaskID: childTaskID(pid, activeTasks), RSSBytes: childRSS, SwapBytes: childSwap, CPUTimeMS: cpuMS, ElapsedMS: elapsedMS})
	}
	return children
}

func sortedTaskIDs(active map[string]struct{}) []string {
	out := make([]string, 0, len(active))
	for id := range active {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func totalChildRSS(children []RunLoopChildResource) uint64 {
	var total uint64
	for _, child := range children {
		total += child.RSSBytes
	}
	return total
}

func childTaskID(pid int, activeTasks []string) string {
	if len(activeTasks) == 1 {
		return activeTasks[0]
	}
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/cmdline")
	if err != nil {
		return ""
	}
	command := string(data)
	for _, id := range activeTasks {
		if strings.Contains(command, id) {
			return id
		}
	}
	return ""
}

func processTimes(pid int) (cpuMS, elapsedMS uint64) {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return 0, 0
	}
	text := string(data)
	end := strings.LastIndexByte(text, ')')
	if end < 0 {
		return 0, 0
	}
	fields := strings.Fields(text[end+1:])
	if len(fields) < 20 {
		return 0, 0
	}
	userTicks, errUser := strconv.ParseUint(fields[11], 10, 64)
	systemTicks, errSystem := strconv.ParseUint(fields[12], 10, 64)
	startTicks, errStart := strconv.ParseUint(fields[19], 10, 64)
	if errUser != nil || errSystem != nil || errStart != nil {
		return 0, 0
	}
	cpuMS = (userTicks + systemTicks) * 10
	uptimeData, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return cpuMS, 0
	}
	uptimeFields := strings.Fields(string(uptimeData))
	if len(uptimeFields) == 0 {
		return cpuMS, 0
	}
	uptimeSeconds, err := strconv.ParseFloat(uptimeFields[0], 64)
	if err == nil && uptimeSeconds*100 > float64(startTicks) {
		elapsedMS = uint64((uptimeSeconds - float64(startTicks)/100) * 1000)
	}
	return cpuMS, elapsedMS
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

// SummarizeParallelismThrottle reports whether memory pressure reduced the
// run-loop parallelism below the requested level, based on the retained
// diagnostics history.
func SummarizeParallelismThrottle(history RunLoopResourceHistory) ParallelismThrottle {
	var summary ParallelismThrottle
	for _, sample := range history.Samples {
		if sample.Phase == "parallelism.throttled" {
			summary.Events++
			summary.LastAt = sample.Timestamp
			if sample.MemoryAvailableBytes > 0 {
				summary.MemoryAvailableBytes = sample.MemoryAvailableBytes
			}
		}
		if sample.RequestedParallelism > 0 {
			summary.RequestedParallelism = sample.RequestedParallelism
		}
		if sample.EffectiveParallelism > 0 {
			summary.EffectiveParallelism = sample.EffectiveParallelism
		}
	}
	summary.Decreased = summary.EffectiveParallelism > 0 && summary.EffectiveParallelism < summary.RequestedParallelism
	return summary
}

func LoadRunLoopResourceHistory(study Study, limit int) (RunLoopResourceHistory, error) {
	if limit < 1 || limit > 1000 {
		limit = 240
	}
	path := filepath.Join(study.Path, RunStateDirName, "diagnostics", "run-loop-memory.jsonl")
	samples := make([]RunLoopMemorySample, 0, limit)
	for _, candidate := range []string{path + ".1", path} {
		file, err := os.Open(candidate)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return RunLoopResourceHistory{}, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
		for scanner.Scan() {
			var sample RunLoopMemorySample
			if json.Unmarshal(scanner.Bytes(), &sample) == nil {
				samples = append(samples, sample)
			}
		}
		scanErr := scanner.Err()
		_ = file.Close()
		if scanErr != nil {
			return RunLoopResourceHistory{}, scanErr
		}
	}
	if len(samples) > limit {
		samples = append([]RunLoopMemorySample(nil), samples[len(samples)-limit:]...)
	}
	return RunLoopResourceHistory{Study: study.Name, Samples: samples}, nil
}
