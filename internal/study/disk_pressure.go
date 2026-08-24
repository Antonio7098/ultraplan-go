package study

import "syscall"

const (
	minimumRuntimeFreeBytes  = 1536 * 1024 * 1024
	criticalRuntimeFreeBytes = 768 * 1024 * 1024
	runtimeWorkerDiskBudget  = 512 * 1024 * 1024
)

type diskPressure struct {
	TotalBytes     uint64
	AvailableBytes uint64
	UsedPercent    float64
	Pressured      bool
	Critical       bool
}

var readSchedulerDiskPressure = readDiskPressure

func diskParallelismCap(pressure diskPressure, requested int) int {
	if requested < 1 || pressure.TotalBytes == 0 {
		return requested
	}
	if pressure.AvailableBytes <= minimumRuntimeFreeBytes {
		return 0
	}
	cap := int((pressure.AvailableBytes - minimumRuntimeFreeBytes) / runtimeWorkerDiskBudget)
	if cap < 1 {
		cap = 1
	}
	if cap > requested {
		cap = requested
	}
	return cap
}

func readDiskPressure(path string) diskPressure {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil || stat.Blocks == 0 {
		return diskPressure{}
	}
	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	used := total - stat.Bfree*uint64(stat.Bsize)
	percent := float64(used) / float64(total) * 100
	return diskPressure{
		TotalBytes: total, AvailableBytes: available, UsedPercent: percent,
		Pressured: available < minimumRuntimeFreeBytes || percent >= 90,
		Critical:  available < criticalRuntimeFreeBytes || percent >= 97,
	}
}
