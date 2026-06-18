package socks5

import (
	"context"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/forward"
	"github.com/paqetpremium/paqetpremium/internal/ioext"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/tunnelpool"
	"github.com/txthinking/socks5"
	"github.com/xtaci/smux"
)

type Server struct {
	route forward.RouteFn
	log   *slog.Logger
}

func New(route forward.RouteFn, log *slog.Logger) *Server {
	return &Server{route: route, log: log}
}

func (s *Server) Start(ctx context.Context, rules []config.SOCKS5Rule) error {
	for _, rule := range rules {
		rule := rule
		go s.listen(ctx, rule)
	}
	return nil
}

func (s *Server) listen(ctx context.Context, rule config.SOCKS5Rule) {
	addr, err := net.ResolveTCPAddr("tcp", rule.Listen)
	if err != nil {
		s.log.Error("socks5 resolve", "listen", rule.Listen, "err", err)
		return
	}

	user, pass := "", ""
	if rule.Auth != nil {
		user = rule.Auth.User
		pass = rule.Auth.Pass
	}

	srv, err := socks5.NewClassicServer(addr.String(), addr.IP.String(), user, pass, 0, 60)
	if err != nil {
		s.log.Error("socks5 create", "listen", rule.Listen, "err", err)
		return
	}
	srv.LimitUDP = true

	handler := &handler{
		route:   s.route,
		log:     s.log,
		ctx:     ctx,
		metrics: metrics.Default,
		pumps:   make(map[uint64]struct{}),
	}

	go func() {
		if err := srv.ListenAndServe(handler); err != nil {
			s.log.Debug("socks5 stopped", "listen", rule.Listen, "err", err)
		}
	}()

	s.log.Info("socks5 listening", "listen", rule.Listen, "udp", true)
	<-ctx.Done()
	_ = srv.Shutdown()
}

type handler struct {
	route   forward.RouteFn
	log     *slog.Logger
	ctx     context.Context
	metrics *metrics.Collector

	pumpMu sync.Mutex
	pumps  map[uint64]struct{}
}

func (h *handler) TCPHandle(s *socks5.Server, conn *net.TCPConn, r *socks5.Request) error {
	switch r.Cmd {
	case socks5.CmdUDP:
		return h.handleUDPAssociate(s, conn, r)
	case socks5.CmdConnect:
		return h.handleTCPConnect(conn, r)
	default:
		return nil
	}
}

func (h *handler) handleUDPAssociate(s *socks5.Server, conn *net.TCPConn, r *socks5.Request) error {
	caddr, err := r.UDP(conn, s.ServerAddr)
	if err != nil {
		return err
	}

	ch := make(chan byte)
	hkey := caddr.String()
	s.AssociatedUDP.Set(hkey, ch, -1)
	defer func() {
		close(ch)
		s.AssociatedUDP.Delete(hkey)
	}()

	h.log.Debug("socks5 udp associate", "client", hkey)

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		close(done)
	}()

	select {
	case <-h.ctx.Done():
	case <-done:
	}
	return nil
}

func (h *handler) handleTCPConnect(conn *net.TCPConn, r *socks5.Request) error {
	reply := []byte{socks5.Ver, socks5.RepSuccess, 0x00}
	laddr := conn.LocalAddr().(*net.TCPAddr)
	if ip4 := laddr.IP.To4(); ip4 != nil {
		reply = append(reply, socks5.ATYPIPv4)
		reply = append(reply, ip4...)
	} else {
		ip6 := laddr.IP.To16()
		reply = append(reply, socks5.ATYPIPv6)
		reply = append(reply, ip6...)
	}
	reply = append(reply, byte(laddr.Port>>8), byte(laddr.Port&0xff))
	if _, err := conn.Write(reply); err != nil {
		return err
	}

	op := h.route("")
	if op == nil {
		return nil
	}

	strm, err := op.OpenTCP(r.Address())
	if err != nil {
		if h.metrics != nil {
			h.metrics.IncError()
		}
		h.log.Warn("socks5 stream", "dest", r.Address(), "err", err)
		return err
	}
	defer strm.Close()

	if h.metrics != nil {
		h.metrics.TCPAccepted.Add(1)
		h.metrics.TCPActive.Add(1)
		defer h.metrics.TCPActive.Add(-1)
	}

	errCh := make(chan error, 2)
	go func() { errCh <- ioext.CopyMetered(conn, strm, h.metrics, false) }()
	go func() { errCh <- ioext.CopyMetered(strm, conn, h.metrics, true) }()

	select {
	case <-h.ctx.Done():
	case err := <-errCh:
		if err != nil {
			h.log.Debug("socks5 closed", "dest", r.Address(), "err", err)
		}
	}
	return nil
}

func (h *handler) UDPHandle(s *socks5.Server, addr *net.UDPAddr, d *socks5.Datagram) error {
	if s.LimitUDP {
		if _, ok := s.AssociatedUDP.Get(addr.String()); !ok {
			return nil
		}
	}

	op := h.route("")
	if op == nil {
		return nil
	}

	target := d.Address()
	local := addr.String()

	strm, isNew, key, err := op.OpenUDP(local, target)
	if err != nil {
		h.log.Warn("socks5 udp stream", "dest", target, "err", err)
		op.CloseUDP(key)
		return err
	}

	if _, err := strm.Write(d.Data); err != nil {
		if h.metrics != nil {
			h.metrics.IncError()
		}
		h.log.Warn("socks5 udp write", "dest", target, "err", err)
		op.CloseUDP(key)
		return err
	}
	if h.metrics != nil {
		h.metrics.UDPPackets.Add(1)
		h.metrics.BytesOut.Add(uint64(len(d.Data)))
	}

	if isNew && h.startPump(key) {
		go h.pumpUDP(s, addr, target, strm, key, op)
	}
	return nil
}

func (h *handler) startPump(key uint64) bool {
	h.pumpMu.Lock()
	defer h.pumpMu.Unlock()
	if _, ok := h.pumps[key]; ok {
		return false
	}
	h.pumps[key] = struct{}{}
	return true
}

func (h *handler) endPump(key uint64) {
	h.pumpMu.Lock()
	delete(h.pumps, key)
	h.pumpMu.Unlock()
}

func (h *handler) pumpUDP(s *socks5.Server, client *net.UDPAddr, target string, strm *smux.Stream, key uint64, op tunnelpool.Opener) {
	defer h.endPump(key)
	defer op.CloseUDP(key)

	atyp, dstAddr, dstPort, err := socks5.ParseAddress(target)
	if err != nil {
		h.log.Warn("socks5 udp parse target", "dest", target, "err", err)
		return
	}
	if atyp == socks5.ATYPDomain {
		dstAddr = dstAddr[1:]
	}

	buf := make([]byte, 64*1024)
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
		}

		_ = strm.SetReadDeadline(time.Now().Add(8 * time.Second))
		n, err := strm.Read(buf)
		_ = strm.SetReadDeadline(time.Time{})
		if err != nil {
			return
		}

		dg := socks5.NewDatagram(atyp, dstAddr, dstPort, buf[:n])
		if _, err := s.UDPConn.WriteToUDP(dg.Bytes(), client); err != nil {
			return
		}
		if h.metrics != nil {
			h.metrics.BytesIn.Add(uint64(n))
		}
	}
}
