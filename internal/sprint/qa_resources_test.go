package sprint

import "testing"

func TestQAParallelismReservesHostMemory(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		available int64
		want      int
	}{
		{name: "invalid request", requested: 0, available: 16 << 30, want: 0},
		{name: "memory pressure", requested: 8, available: 2 << 30, want: 1},
		{name: "small laptop", requested: 8, available: 4 << 30, want: 2},
		{name: "configured cap", requested: 3, available: 32 << 30, want: 3},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := qaParallelismForAvailableMemory(test.requested, test.available); got != test.want {
				t.Fatalf("parallelism = %d, want %d", got, test.want)
			}
		})
	}
}

func TestQAHostAvailableMemoryIsPositiveWhenReported(t *testing.T) {
	if available, ok := qaHostAvailableMemory(); ok && available <= 0 {
		t.Fatalf("available memory = %d", available)
	}
}
