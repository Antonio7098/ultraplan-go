package sprint

import (
	"os"
	"strconv"
	"strings"
)

const (
	qaHostMemoryReserve    int64 = 1 << 30
	qaRuntimeMemoryBudget  int64 = 1536 << 20
	qaMinimumMemoryToStart int64 = qaHostMemoryReserve + qaRuntimeMemoryBudget
)

// qaRuntimeParallelism keeps enough memory outside the QA runtime pool for the
// desktop and UltraPlan itself. Model harnesses are separate processes, so Go
// heap limits do not protect the host from concurrent investigator pressure.
func qaRuntimeParallelism(requested int) int {
	if requested < 1 {
		return 0
	}
	available, ok := qaHostAvailableMemory()
	if !ok {
		// Unknown hosts get the safe default. Operators can still run one model
		// process without pretending that an unmeasured machine can sustain more.
		return 1
	}
	return qaParallelismForAvailableMemory(requested, available)
}

func qaParallelismForAvailableMemory(requested int, available int64) int {
	if requested < 1 {
		return 0
	}
	if available < qaMinimumMemoryToStart {
		return 1
	}
	workers := int((available - qaHostMemoryReserve) / qaRuntimeMemoryBudget)
	if workers < 1 {
		workers = 1
	}
	if workers > requested {
		workers = requested
	}
	return workers
}

func qaHostAvailableMemory() (int64, bool) {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, false
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "MemAvailable:" {
			continue
		}
		kb, err := strconv.ParseInt(fields[1], 10, 64)
		if err != nil || kb <= 0 {
			return 0, false
		}
		return kb * 1024, true
	}
	return 0, false
}
