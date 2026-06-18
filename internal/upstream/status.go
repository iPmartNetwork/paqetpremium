package upstream

type ServerStatus struct {
	Name     string  `json:"name"`
	Addr     string  `json:"addr"`
	Healthy  bool    `json:"healthy"`
	Active   bool    `json:"active"`
	RTTMs    float64 `json:"rtt_ms"`
	Failures int     `json:"failures"`
	Sessions int     `json:"sessions"`
}

func (m *Manager) Strategy() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.strategy
}

func (m *Manager) Snapshot() []ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ServerStatus, 0, len(m.order))
	for _, name := range m.order {
		e, ok := m.entries[name]
		if !ok {
			continue
		}
		rtt := float64(0)
		if e.lastRTT > 0 {
			rtt = float64(e.lastRTT.Microseconds()) / 1000
		}
		sessions := 0
		if e.pool != nil {
			sessions = e.pool.Count()
		}
		out = append(out, ServerStatus{
			Name:     name,
			Addr:     e.ep.Addr.String(),
			Healthy:  e.healthy,
			Active:   name == m.active,
			RTTMs:    rtt,
			Failures: e.failures,
			Sessions: sessions,
		})
	}
	return out
}
