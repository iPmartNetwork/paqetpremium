package metrics

import (
	"io"
	"sync/atomic"
)

// Collector holds process-wide traffic counters (safe for concurrent use).
type Collector struct {
	BytesIn      atomic.Uint64
	BytesOut     atomic.Uint64
	TCPAccepted  atomic.Uint64
	TCPActive    atomic.Int64
	UDPPackets   atomic.Uint64
	RelayTCP     atomic.Uint64
	RelayUDP     atomic.Uint64
	Errors       atomic.Uint64
}

// Default is the shared metrics collector for the running process.
var Default = &Collector{}

type Snapshot struct {
	BytesIn     uint64 `json:"bytes_in"`
	BytesOut    uint64 `json:"bytes_out"`
	TCPAccepted uint64 `json:"tcp_accepted"`
	TCPActive   int64  `json:"tcp_active"`
	UDPPackets  uint64 `json:"udp_packets"`
	RelayTCP    uint64 `json:"relay_tcp"`
	RelayUDP    uint64 `json:"relay_udp"`
	Errors      uint64 `json:"errors"`
}

func (c *Collector) Snapshot() Snapshot {
	if c == nil {
		return Snapshot{}
	}
	return Snapshot{
		BytesIn:     c.BytesIn.Load(),
		BytesOut:    c.BytesOut.Load(),
		TCPAccepted: c.TCPAccepted.Load(),
		TCPActive:   c.TCPActive.Load(),
		UDPPackets:  c.UDPPackets.Load(),
		RelayTCP:    c.RelayTCP.Load(),
		RelayUDP:    c.RelayUDP.Load(),
		Errors:      c.Errors.Load(),
	}
}

func (c *Collector) IncError() {
	if c != nil {
		c.Errors.Add(1)
	}
}

type meteredWriter struct {
	w io.Writer
	n *atomic.Uint64
}

func (m *meteredWriter) Write(p []byte) (int, error) {
	n, err := m.w.Write(p)
	if n > 0 && m.n != nil {
		m.n.Add(uint64(n))
	}
	return n, err
}

func (c *Collector) MeterWriter(w io.Writer, outbound bool) io.Writer {
	if c == nil {
		return w
	}
	if outbound {
		return &meteredWriter{w: w, n: &c.BytesOut}
	}
	return &meteredWriter{w: w, n: &c.BytesIn}
}
