package study

import "syscall"

const (
	minimumRuntimeFreeBytes  = 768 * 1024 * 1024
	criticalRuntimeFreeBytes = 256 * 1024 * 1024
)

type diskPressure struct {
	TotalBytes     uint64
	AvailableBytes uint64
	UsedPercent    float64
	Pressured      bool
	Critical       bool
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
