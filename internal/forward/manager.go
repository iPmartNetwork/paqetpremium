package forward

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/ioext"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/tunnelpool"
	"github.com/xtaci/smux"
)

type Rule struct {
	Listen       string
	Target       string
	Proto        string
	BindUpstream string
}

type Manager struct {
	route   RouteFn
	log     *slog.Logger
	metrics *metrics.Collector
	wg      sync.WaitGroup
}

func NewManager(route RouteFn, log *slog.Logger) *Manager {
	return &Manager{route: route, log: log, metrics: metrics.Default}
}

func (m *Manager) Start(ctx context.Context, rules []Rule) error {
	for _, r := range rules {
		switch r.Proto {
		case "tcp", "":
			m.wg.Add(1)
			go func(rule Rule) {
				defer m.wg.Done()
				m.serveTCP(ctx, rule)
			}(r)
		case "udp":
			m.wg.Add(1)
			go func(rule Rule) {
				defer m.wg.Done()
				m.serveUDP(ctx, rule)
			}(r)
		default:
			return fmt.Errorf("unsupported forward protocol %q", r.Proto)
		}
	}
	return nil
}

func (m *Manager) Wait() { m.wg.Wait() }

func (m *Manager) opener(bind string) tunnelpool.Opener {
	if m.route != nil {
		return m.route(bind)
	}
	return nil
}

func (m *Manager) serveTCP(ctx context.Context, rule Rule) {
	ln, err := net.Listen("tcp", rule.Listen)
	if err != nil {
		m.log.Error("tcp forward listen failed", "listen", rule.Listen, "err", err)
		return
	}
	defer ln.Close()

	go func() {
		<-ctx.Done()
		ln.Close()
	}()

	m.log.Info("tcp forward listening", "listen", rule.Listen, "target", rule.Target, "upstream", rule.BindUpstream)

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
				m.log.Warn("tcp accept", "listen", rule.Listen, "err", err)
				continue
			}
		}

		m.wg.Add(1)
		go func(c net.Conn) {
			defer m.wg.Done()
			defer c.Close()
			if m.metrics != nil {
				m.metrics.TCPAccepted.Add(1)
				m.metrics.TCPActive.Add(1)
				defer m.metrics.TCPActive.Add(-1)
			}
			m.handleTCP(ctx, c, rule)
		}(conn)
	}
}

func (m *Manager) handleTCP(ctx context.Context, conn net.Conn, rule Rule) {
	op := m.opener(rule.BindUpstream)
	if op == nil {
		m.log.Warn("no tunnel opener configured")
		return
	}
	strm, err := op.OpenTCP(rule.Target)
	if err != nil {
		if m.metrics != nil {
			m.metrics.IncError()
		}
		m.log.Warn("tcp forward stream", "target", rule.Target, "err", err)
		return
	}
	defer strm.Close()

	errCh := make(chan error, 2)
	go func() { errCh <- ioext.CopyMetered(strm, conn, m.metrics, true) }()
	go func() { errCh <- ioext.CopyMetered(conn, strm, m.metrics, false) }()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			m.log.Debug("tcp forward closed", "target", rule.Target, "err", err)
		}
	}
}

func (m *Manager) serveUDP(ctx context.Context, rule Rule) {
	laddr, err := net.ResolveUDPAddr("udp", rule.Listen)
	if err != nil {
		m.log.Error("udp forward resolve", "listen", rule.Listen, "err", err)
		return
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		m.log.Error("udp forward listen", "listen", rule.Listen, "err", err)
		return
	}
	defer conn.Close()

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	m.log.Info("udp forward listening", "listen", rule.Listen, "target", rule.Target, "upstream", rule.BindUpstream)
	buf := make([]byte, 64*1024)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, caddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.log.Warn("udp read", "err", err)
			continue
		}
		if n == 0 {
			continue
		}
		if m.metrics != nil {
			m.metrics.UDPPackets.Add(1)
		}

		op := m.opener(rule.BindUpstream)
		if op == nil {
			continue
		}

		strm, isNew, key, err := op.OpenUDP(caddr.String(), rule.Target)
		if err != nil {
			m.log.Warn("udp forward stream", "target", rule.Target, "err", err)
			op.CloseUDP(key)
			continue
		}

		if err := protocol.WriteDatagram(strm, buf[:n]); err != nil {
			if m.metrics != nil {
				m.metrics.IncError()
			}
			m.log.Warn("udp forward write", "err", err)
			op.CloseUDP(key)
			continue
		}
		if m.metrics != nil {
			m.metrics.BytesOut.Add(uint64(n))
		}

		if isNew {
			m.wg.Add(1)
			go func(s *smux.Stream, k uint64, client *net.UDPAddr, bind string) {
				defer m.wg.Done()
				m.pumpUDP(ctx, s, conn, client, k, bind)
			}(strm, key, caddr, rule.BindUpstream)
		}
	}
}

func (m *Manager) pumpUDP(ctx context.Context, strm *smux.Stream, conn *net.UDPConn, client *net.UDPAddr, key uint64, bind string) {
	defer m.opener(bind).CloseUDP(key)
	buf := make([]byte, 64*1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// UDP idle timeout (server->client): allow idle-but-active sessions
		// (e.g. WireGuard without keepalive) to stay open longer.
		_ = strm.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, err := protocol.ReadDatagram(strm, buf)
		_ = strm.SetReadDeadline(time.Time{})
		if err != nil {
			return
		}
		if m.metrics != nil {
			m.metrics.BytesIn.Add(uint64(n))
		}
		if _, err := conn.WriteToUDP(buf[:n], client); err != nil {
			return
		}
	}
}
