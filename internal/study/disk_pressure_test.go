package study

import "testing"

func TestDiskParallelismCapReservesHeadroomPerWorker(t *testing.T) {
	tests := []struct {
		name      string
		total     uint64
		available uint64
		requested int
		want      int
	}{
		{name: "unknown", available: 0, requested: 4, want: 4},
		{name: "paused", total: 10 << 30, available: minimumRuntimeFreeBytes, requested: 4, want: 0},
		{name: "one", total: 10 << 30, available: minimumRuntimeFreeBytes + runtimeWorkerDiskBudget, requested: 4, want: 1},
		{name: "bounded", total: 10 << 30, available: minimumRuntimeFreeBytes + 2*runtimeWorkerDiskBudget, requested: 4, want: 2},
		{name: "requested", total: 10 << 30, available: minimumRuntimeFreeBytes + 8*runtimeWorkerDiskBudget, requested: 4, want: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := diskParallelismCap(diskPressure{TotalBytes: test.total, AvailableBytes: test.available}, test.requested)
			if got != test.want {
				t.Fatalf("cap = %d, want %d", got, test.want)
			}
		})
	}
}
