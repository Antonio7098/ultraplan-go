package study

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

const gibibyte = uint64(1024 * 1024 * 1024)

type memoryPressure struct {
	AvailableBytes uint64
	Stretched      bool
	Recovered      bool
}

func readMemoryPressure() memoryPressure {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return memoryPressure{}
	}
	defer file.Close()
	values := map[string]uint64{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err == nil {
			values[strings.TrimSuffix(fields[0], ":")] = value * 1024
		}
	}
	total := values["MemTotal"]
	available := values["MemAvailable"]
	if total == 0 || available == 0 {
		return memoryPressure{}
	}
	stretchThreshold := total * 15 / 100
	if stretchThreshold < gibibyte {
		stretchThreshold = gibibyte
	}
	recoveryThreshold := total * 25 / 100
	if recoveryThreshold < gibibyte+gibibyte/2 {
		recoveryThreshold = gibibyte + gibibyte/2
	}
	return memoryPressure{AvailableBytes: available, Stretched: available < stretchThreshold, Recovered: available > recoveryThreshold}
}
