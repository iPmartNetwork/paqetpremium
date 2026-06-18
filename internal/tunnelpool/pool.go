package tunnelpool

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/paqetpremium/paqetpremium/internal/netutil"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
	"github.com/paqetpremium/paqetpremium/internal/tunneladdr"
	"github.com/xtaci/smux"
)

type Pool struct {
	sessions []*transport.Session
	idx      uint32

	udpMu sync.RWMutex
	udp   map[uint64]*smux.Stream
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
	for _, s := range p.sessions {
		if s != nil {
			s.Close()
		}
	}
	p.sessions = nil
}

func (p *Pool) openStream() (*smux.Stream, error) {
	if len(p.sessions) == 0 {
		return nil, fmt.Errorf("no tunnel sessions")
	}
	i := atomic.AddUint32(&p.idx, 1) - 1
	sess := p.sessions[int(i)%len(p.sessions)]
	return sess.Smux.OpenStream()
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
