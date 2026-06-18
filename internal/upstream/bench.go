package upstream

import (
	"context"
	"time"
)

type BenchResult struct {
	Name   string        `json:"name"`
	Addr   string        `json:"addr"`
	RTT    time.Duration `json:"rtt"`
	OK     bool          `json:"ok"`
	Error  string        `json:"error,omitempty"`
}

func (m *Manager) Benchmark(ctx context.Context, timeout time.Duration) []BenchResult {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]BenchResult, 0, len(m.order))
	for _, name := range m.order {
		e, ok := m.entries[name]
		if !ok {
			continue
		}
		br := BenchResult{Name: name, Addr: e.ep.Addr.String()}
		start := time.Now()
		err := e.pool.PingWithTimeout(timeout)
		br.RTT = time.Since(start)
		if err != nil {
			br.Error = err.Error()
		} else {
			br.OK = true
		}
		out = append(out, br)
	}
	return out
}
