package tunnelpool

import (
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/paqetpremium/paqetpremium/internal/netutil"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
	"github.com/paqetpremium/paqetpremium/internal/tunneladdr"
	"github.com/xtaci/smux"
)

// DatagramOpener is implemented by openers/pools that can carry UDP forward
// traffic over unreliable QUIC datagrams instead of a reliable smux stream.
type DatagramOpener interface {
	SupportsDatagrams() bool
	OpenUDPDatagram(key, target string, deliver func([]byte)) (send func([]byte) error, isNew bool, err error)
}

type Pool struct {
	ctx context.Context

	sessions []*transport.Session
	idx      uint32

	udpMu sync.RWMutex
	udp   map[uint64]*smux.Stream

	dgMu      sync.Mutex
	dgIdx     uint32
	dgByKey   map[string]func([]byte) error
	dgCtrl    map[string]*smux.Stream
	dgDeliver map[*transport.Session]map[uint64]func([]byte)
	dgRecv    map[*transport.Session]bool
}

func (p *Pool) Count() int { return len(p.sessions) }

func (p *Pool) sendRemoteFlags(sess *transport.Session, flags []netutil.TCPFlagSet) error {
	strm, err := sess.Smux.OpenStream()
	if err != nil {
		return err
	}
	defer strm.Close()
	return (&protocol.Message{Type: protocol.TCPF, TCPF: flags}).Write(strm)
}

func (p *Pool) Close() {
	p.dgMu.Lock()
	for _, ctrl := range p.dgCtrl {
		if ctrl != nil {
			ctrl.Close()
		}
	}
	p.dgCtrl = nil
	p.dgMu.Unlock()
	for _, s := range p.sessions {
		if s != nil {
			s.Close()
		}
	}
	p.sessions = nil
}

// SupportsDatagrams reports whether any live session can carry QUIC datagrams.
func (p *Pool) SupportsDatagrams() bool {
	for _, s := range p.sessions {
		if s != nil && s.DatagramsOK() && s.Smux != nil && !s.Smux.IsClosed() {
			return true
		}
	}
	return false
}

// pickQUICSession round-robins over datagram-capable, live sessions.
func (p *Pool) pickQUICSession() *transport.Session {
	n := len(p.sessions)
	if n == 0 {
		return nil
	}
	start := int(atomic.AddUint32(&p.dgIdx, 1) - 1)
	for off := 0; off < n; off++ {
		sess := p.sessions[(start+off)%n]
		if sess == nil || !sess.DatagramsOK() {
			continue
		}
		if sess.Smux == nil || sess.Smux.IsClosed() {
			continue
		}
		return sess
	}
	return nil
}

// OpenUDPDatagram establishes (or reuses) a QUIC-datagram UDP flow keyed by key.
// On a new flow it opens a control smux stream, requests the target, reads the
// server-assigned 8-byte flowID, and registers deliver for inbound payloads.
// It returns a send closure that frames and sends payloads over the datagram.
func (p *Pool) OpenUDPDatagram(key, target string, deliver func([]byte)) (func([]byte) error, bool, error) {
	p.dgMu.Lock()
	defer p.dgMu.Unlock()

	if send, ok := p.dgByKey[key]; ok {
		return send, false, nil
	}

	sess := p.pickQUICSession()
	if sess == nil {
		return nil, false, fmt.Errorf("no quic session")
	}

	addr, err := tunneladdr.Parse(target)
	if err != nil {
		return nil, false, err
	}
	ctrl, err := sess.Smux.OpenStream()
	if err != nil {
		return nil, false, err
	}
	if err := (&protocol.Message{Type: protocol.UDPDGRAM, Addr: addr}).Write(ctrl); err != nil {
		ctrl.Close()
		return nil, false, err
	}
	var flowID uint64
	if err := binary.Read(ctrl, binary.BigEndian, &flowID); err != nil {
		ctrl.Close()
		return nil, false, err
	}

	if p.dgDeliver[sess] == nil {
		p.dgDeliver[sess] = make(map[uint64]func([]byte))
	}
	p.dgDeliver[sess][flowID] = deliver
	if !p.dgRecv[sess] {
		p.dgRecv[sess] = true
		go p.dgramRecvLoop(sess)
	}

	send := func(payload []byte) error {
		return sess.SendDatagram(protocol.EncodeDatagram(flowID, payload))
	}
	p.dgByKey[key] = send
	p.dgCtrl[key] = ctrl
	return send, true, nil
}

// dgramRecvLoop drains inbound QUIC datagrams for a session and dispatches each
// decoded payload to the deliver closure registered for its flowID.
func (p *Pool) dgramRecvLoop(sess *transport.Session) {
	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		b, err := sess.ReceiveDatagram(ctx)
		if err != nil {
			return
		}
		flowID, payload, ok := protocol.DecodeDatagram(b)
		if !ok {
			continue
		}
		p.dgMu.Lock()
		var deliver func([]byte)
		if m := p.dgDeliver[sess]; m != nil {
			deliver = m[flowID]
		}
		p.dgMu.Unlock()
		if deliver == nil {
			continue
		}
		// Copy: the received buffer may be reused by the next read.
		cp := make([]byte, len(payload))
		copy(cp, payload)
		deliver(cp)
	}
}

func (p *Pool) openStream() (*smux.Stream, error) {
	n := len(p.sessions)
	if n == 0 {
		return nil, fmt.Errorf("no tunnel sessions")
	}
	start := int(atomic.AddUint32(&p.idx, 1) - 1)
	var lastErr error
	for off := 0; off < n; off++ {
		sess := p.sessions[(start+off)%n]
		if sess == nil || sess.Smux == nil || sess.Smux.IsClosed() {
			continue
		}
		strm, err := sess.Smux.OpenStream()
		if err != nil {
			lastErr = err
			continue
		}
		return strm, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("no live tunnel sessions")
}

// Alive reports whether the pool has at least one open session.
func (p *Pool) Alive() bool {
	for _, s := range p.sessions {
		if s != nil && s.Smux != nil && !s.Smux.IsClosed() {
			return true
		}
	}
	return false
}

func (p *Pool) OpenTCP(target string) (*smux.Stream, error) {
	addr, err := tunneladdr.Parse(target)
	if err != nil {
		return nil, err
	}
	strm, err := p.openStream()
	if err != nil {
		return nil, err
	}
	if err := (&protocol.Message{Type: protocol.TCP, Addr: addr}).Write(strm); err != nil {
		strm.Close()
		return nil, err
	}
	return strm, nil
}

func (p *Pool) OpenUDP(localAddr, target string) (*smux.Stream, bool, uint64, error) {
	key := addrPairKey(localAddr, target)

	p.udpMu.RLock()
	if strm, ok := p.udp[key]; ok {
		p.udpMu.RUnlock()
		return strm, false, key, nil
	}
	p.udpMu.RUnlock()

	addr, err := tunneladdr.Parse(target)
	if err != nil {
		return nil, false, 0, err
	}
	strm, err := p.openStream()
	if err != nil {
		return nil, false, 0, err
	}
	if err := (&protocol.Message{Type: protocol.UDP, Addr: addr}).Write(strm); err != nil {
		strm.Close()
		return nil, false, 0, err
	}

	p.udpMu.Lock()
	p.udp[key] = strm
	p.udpMu.Unlock()
	return strm, true, key, nil
}

func (p *Pool) CloseUDP(key uint64) {
	p.udpMu.Lock()
	if strm, ok := p.udp[key]; ok {
		strm.Close()
		delete(p.udp, key)
	}
	p.udpMu.Unlock()
}

func addrPairKey(a, b string) uint64 {
	h := uint64(2166136261)
	for _, s := range []string{a, b} {
		for i := 0; i < len(s); i++ {
			h ^= uint64(s[i])
			h *= 16777619
		}
	}
	return h
}
