package runcontrol

import (
	"sync/atomic"
	"time"
)

// OperationMetric is a bounded process-local latency/counter projection. It
// deliberately contains no identity or target labels.
type OperationMetric struct {
	Count             uint64 `json:"count"`
	Failures          uint64 `json:"failures"`
	TotalMicroseconds uint64 `json:"total_microseconds"`
	MaxMicroseconds   uint64 `json:"max_microseconds"`
}

type LocalMetrics struct {
	Acceptance           OperationMetric `json:"acceptance"`
	Append               OperationMetric `json:"append"`
	Terminal             OperationMetric `json:"terminal"`
	LeaseRenewals        uint64          `json:"lease_renewals"`
	CancellationRequests uint64          `json:"cancellation_requests"`
	ReconciliationScans  uint64          `json:"reconciliation_scans"`
	CompactionPasses     uint64          `json:"compaction_passes"`
}

type atomicOperationMetric struct {
	count, failures, totalMicros, maxMicros atomic.Uint64
}

func (m *atomicOperationMetric) observe(start time.Time, err error) {
	m.count.Add(1)
	if err != nil {
		m.failures.Add(1)
	}
	micros := uint64(time.Since(start).Microseconds())
	m.totalMicros.Add(micros)
	for current := m.maxMicros.Load(); micros > current && !m.maxMicros.CompareAndSwap(current, micros); current = m.maxMicros.Load() {
	}
}

func (m *atomicOperationMetric) snapshot() OperationMetric {
	return OperationMetric{Count: m.count.Load(), Failures: m.failures.Load(), TotalMicroseconds: m.totalMicros.Load(), MaxMicroseconds: m.maxMicros.Load()}
}

type repositoryMetrics struct {
	acceptance, append, terminal                                      atomicOperationMetric
	leaseRenewals, cancellationRequests, reconciliations, compactions atomic.Uint64
}

func (m *repositoryMetrics) snapshot() LocalMetrics {
	return LocalMetrics{
		Acceptance: m.acceptance.snapshot(), Append: m.append.snapshot(), Terminal: m.terminal.snapshot(),
		LeaseRenewals: m.leaseRenewals.Load(), CancellationRequests: m.cancellationRequests.Load(),
		ReconciliationScans: m.reconciliations.Load(), CompactionPasses: m.compactions.Load(),
	}
}
