package tunnel

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
)

// dgramHub multiplexes QUIC DATAGRAM UDP flows for a single tunnel session.
// Each flow maps a uint64 flowID to a dialed UDP socket toward the target.
// The client->target direction is dispatched by a single recvLoop per session;
// the target->client direction runs one goroutine per flow.
type dgramHub struct {
	sess   *transport.Session
	log    *slog.Logger
	mu     sync.Mutex
	flows  map[uint64]net.Conn
	nextID atomic.Uint64
}

func newDgramHub(sess *transport.Session, log *slog.Logger) *dgramHub {
	return &dgramHub{
		sess:  sess,
		log:   log,
		flows: make(map[uint64]net.Conn),
	}
}

// recvLoop is the per-session CLIENT->TARGET dispatcher. It reads QUIC
// datagrams, decodes the flowID, and writes the payload to the matching UDP
// socket. It returns when the session can no longer receive datagrams.
func (h *dgramHub) recvLoop(ctx context.Context) {
	for {
		b, err := h.sess.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		flowID, payload, ok := protocol.DecodeDatagram(b)
		if !ok {
			continue
		}
		h.mu.Lock()
		conn := h.flows[flowID]
		h.mu.Unlock()
		if conn == nil {
			continue
		}
		// Best-effort per-packet write; UDP is unreliable so ignore errors.
		if _, err := conn.Write(payload); err == nil {
			metrics.Default.UDPDgramIn.Add(1)
			metrics.Default.BytesOut.Add(uint64(len(payload)))
		}
	}
}

// openFlow dials the UDP target, allocates a flowID, and registers the flow.
func (h *dgramHub) openFlow(target string) (uint64, net.Conn, error) {
	conn, err := net.Dial("udp", target)
	if err != nil {
		metrics.Default.IncError()
		return 0, nil, err
	}
	metrics.Default.RelayUDP.Add(1)
	metrics.Default.UDPDgramFlows.Add(1)
	flowID := h.nextID.Add(1)
	h.mu.Lock()
	h.flows[flowID] = conn
	h.mu.Unlock()
	return flowID, conn, nil
}

// flowReadTargetToClient is the per-flow TARGET->CLIENT loop. It reads from the
// UDP socket and forwards each datagram to the client over QUIC. Oversized
// payloads are dropped and counted. On exit the flow is removed and closed.
func (h *dgramHub) flowReadTargetToClient(ctx context.Context, flowID uint64, conn net.Conn) {
	defer h.closeFlow(flowID)
	buf := make([]byte, 64*1024)
	max := h.sess.MaxDatagramPayload()
	for {
		if ctx.Err() != nil {
			return
		}
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, err := conn.Read(buf)
		if err != nil {
			return
		}
		if n > max {
			metrics.Default.UDPDgramDropped.Add(1)
			continue
		}
		if err := h.sess.SendDatagram(protocol.EncodeDatagram(flowID, buf[:n])); err != nil {
			metrics.Default.UDPDgramDropped.Add(1)
			continue
		}
		metrics.Default.UDPDgramOut.Add(1)
		metrics.Default.BytesIn.Add(uint64(n))
	}
}

// closeFlow closes and removes a flow's UDP socket. Safe to call repeatedly.
func (h *dgramHub) closeFlow(flowID uint64) {
	h.mu.Lock()
	conn := h.flows[flowID]
	delete(h.flows, flowID)
	h.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}
