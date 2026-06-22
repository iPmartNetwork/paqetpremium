package tunnel

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/paqetpremium/paqetpremium/internal/admin"
	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/iptables"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/pcap"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
	"github.com/paqetpremium/paqetpremium/internal/transport"
	"github.com/paqetpremium/paqetpremium/internal/version"
	"github.com/xtaci/smux"
)

type Server struct {
	cfgPath string
	cfg     atomic.Pointer[config.Config]
	log     *slog.Logger

	sessions atomic.Int64
}

func NewServer(cfg *config.Config, cfgPath string, log *slog.Logger) *Server {
	s := &Server{cfgPath: cfgPath, log: log}
	s.cfg.Store(cfg)
	return s
}

func (s *Server) Run(ctx context.Context) error {
	cfg := s.cfg.Load()
	netRT, err := cfg.ResolveNetwork()
	if err != nil {
		return err
	}

	if err := iptables.Apply(netRT); err != nil {
		s.log.Warn("iptables setup failed (run as root on server)", "err", err)
	} else {
		s.log.Info("iptables rules applied", "port", netRT.Port, "ipv6", netRT.IPv6 != nil)
		defer func() { _ = iptables.Remove() }()
	}

	opt, err := transport.OptionsFromConfig(config.RoleServer, cfg.Transport)
	if err != nil {
		return err
	}

	pconn, err := pcap.Open(ctx, netRT)
	if err != nil {
		return fmt.Errorf("open pcap: %w", err)
	}

	ln, err := transport.Listen(opt, pconn)
	if err != nil {
		return fmt.Errorf("%s listen: %w", opt.Protocol, err)
	}
	defer ln.Close()

	s.log.Info("server listening",
		"port", netRT.Port,
		"interface", netRT.Interface.Name,
		"protocol", opt.Protocol,
	)

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go s.watchReload(runCtx)
	s.startAdmin(runCtx, netRT.Port)

	var wg sync.WaitGroup
	go func() {
		<-runCtx.Done()
		ln.Close()
	}()

	for {
		if runCtx.Err() != nil {
			wg.Wait()
			return nil
		}

		sess, err := ln.Accept(runCtx)
		if err != nil {
			if runCtx.Err() != nil {
				return nil
			}
			s.log.Warn("accept failed", "err", err)
			continue
		}

		wg.Add(1)
		go func(sess *transport.Session) {
			defer wg.Done()
			defer sess.Close()
			s.sessions.Add(1)
			defer s.sessions.Add(-1)
			s.serveSession(runCtx, sess, pconn)
		}(sess)
	}
}

func (s *Server) watchReload(ctx context.Context) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	for {
		select {
		case <-ctx.Done():
			return
		case <-sigCh:
			if err := s.ReloadFromDisk(); err != nil {
				s.log.Error("reload failed", "err", err)
			}
		}
	}
}

func (s *Server) ReloadFromDisk() error {
	if s.cfgPath == "" {
		return fmt.Errorf("config path unknown")
	}
	old := s.cfg.Load()
	cfg, err := config.Load(s.cfgPath)
	if err != nil {
		return err
	}

	oldRT, err := old.ResolveNetwork()
	if err != nil {
		return err
	}
	newRT, err := cfg.ResolveNetwork()
	if err != nil {
		return err
	}

	if old.Transport.Protocol != cfg.Transport.Protocol {
		return fmt.Errorf("reload cannot change transport.protocol; restart required")
	}
	if old.Transport.KCP.Key != cfg.Transport.KCP.Key {
		return fmt.Errorf("reload cannot change transport.kcp.key; restart required")
	}
	if oldRT.Port != newRT.Port {
		return fmt.Errorf("reload cannot change tunnel port; restart required")
	}
	if old.Network.Interface != cfg.Network.Interface {
		return fmt.Errorf("reload cannot change network.interface; restart required")
	}

	if err := iptables.Apply(newRT); err != nil {
		s.log.Warn("iptables reload failed", "err", err)
	} else {
		s.log.Info("iptables rules refreshed", "port", newRT.Port, "ipv6", newRT.IPv6 != nil)
	}

	s.cfg.Store(cfg)
	s.log.Info("server config reloaded",
		"file", s.cfgPath,
		"name", cfg.Name,
	)
	return nil
}

func (s *Server) serveSession(ctx context.Context, sess *transport.Session, pconn *pcap.Conn) {
	remote := sess.RemoteAddr()
	s.log.Info("tunnel session accepted", "remote", remote)

	var hub *dgramHub
	if sess.DatagramsOK() {
		hub = newDgramHub(sess, s.log)
		go hub.recvLoop(ctx)
	}

	for {
		if ctx.Err() != nil {
			return
		}

		strm, err := sess.Smux.AcceptStream()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Debug("smux accept ended", "err", err)
			return
		}

		go s.handleStream(ctx, strm, pconn, remote, hub)
	}
}

func (s *Server) handleStream(ctx context.Context, strm *smux.Stream, pconn *pcap.Conn, remote net.Addr, hub *dgramHub) {
	defer strm.Close()

	var msg protocol.Message
	if err := msg.Read(strm); err != nil {
		s.log.Debug("protocol read", "err", err)
		return
	}

	switch msg.Type {
	case protocol.Ping:
		_ = (&protocol.Message{Type: protocol.Pong}).Write(strm)
	case protocol.TCPF:
		if pconn != nil && len(msg.TCPF) > 0 {
			pconn.SetRemoteTCPFlags(remote, msg.TCPF)
		}
	case protocol.TCP:
		if msg.Addr == nil {
			return
		}
		s.log.Info("tcp relay", "dest", msg.Addr.String(), "remote", remote)
		if err := relayTCP(ctx, strm, msg.Addr.String()); err != nil {
			s.log.Debug("tcp relay done", "dest", msg.Addr.String(), "err", err)
		}
	case protocol.UDP:
		if msg.Addr == nil {
			return
		}
		s.log.Info("udp relay", "dest", msg.Addr.String(), "remote", remote)
		if err := relayUDP(ctx, strm, msg.Addr.String()); err != nil {
			s.log.Debug("udp relay done", "dest", msg.Addr.String(), "err", err)
		}
	case protocol.UDPDGRAM:
		if hub == nil || msg.Addr == nil {
			return
		}
		flowID, conn, err := hub.openFlow(msg.Addr.String())
		if err != nil {
			s.log.Debug("udp datagram dial failed", "dest", msg.Addr.String(), "err", err)
			return
		}
		defer hub.closeFlow(flowID)
		// Reply on the control stream with the assigned 8-byte flowID.
		if err := binary.Write(strm, binary.BigEndian, flowID); err != nil {
			s.log.Debug("udp datagram flowid write failed", "dest", msg.Addr.String(), "err", err)
			return
		}
		s.log.Info("udp datagram relay", "dest", msg.Addr.String(), "remote", remote, "flow", flowID)
		go hub.flowReadTargetToClient(ctx, flowID, conn)
		// Block on the control stream; when the client closes it, tear down.
		_, _ = io.Copy(io.Discard, strm)
	default:
		s.log.Warn("unknown protocol type", "type", msg.Type)
	}
}

func (s *Server) startAdmin(ctx context.Context, listenPort int) {
	cfg := s.cfg.Load()
	if cfg.Admin == nil {
		return
	}
	srv := admin.New(cfg.Admin, s.log, func() admin.Status {
		cfg := s.cfg.Load()
		stats := metrics.Default.Snapshot()
		return admin.Status{
			Core:     version.Name,
			Version:  version.Version,
			Role:     "server",
			Name:     cfg.Name,
			Listen:   fmt.Sprintf("%d", listenPort),
			Sessions: int(s.sessions.Load()),
			Stats:    &stats,
		}
	}, func() error {
		return s.ReloadFromDisk()
	})
	srv.WithConfigEditor(s.cfgPath, func() *config.Config { return s.cfg.Load() })
	go func() {
		if err := srv.Run(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("admin server failed", "err", err)
		}
	}()
}
