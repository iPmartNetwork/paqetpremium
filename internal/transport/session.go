package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/pcap"
	"github.com/quic-go/quic-go"
	"github.com/xtaci/kcp-go/v5"
	"github.com/xtaci/smux"
)

type Session struct {
	PacketConn *pcap.Conn
	remoteAddr net.Addr

	KCP        *kcp.UDPSession
	quicTr     *quic.Transport
	quicConn   *quic.Conn
	quicStream *quic.Stream

	Smux *smux.Session
}

func (s *Session) RemoteAddr() net.Addr {
	if s.remoteAddr != nil {
		return s.remoteAddr
	}
	if s.KCP != nil {
		return s.KCP.RemoteAddr()
	}
	if s.quicConn != nil {
		return s.quicConn.RemoteAddr()
	}
	return nil
}

func (s *Session) Close() error {
	if s.Smux != nil {
		s.Smux.Close()
	}
	if s.quicStream != nil {
		_ = s.quicStream.Close()
	}
	if s.quicConn != nil {
		_ = s.quicConn.CloseWithError(0, "close")
	}
	if s.quicTr != nil {
		_ = s.quicTr.Close()
	}
	if s.KCP != nil {
		s.KCP.Close()
	}
	if s.PacketConn != nil {
		s.PacketConn.Close()
	}
	return nil
}

func smuxConfig(opt Options) *smux.Config {
	cfg := smux.DefaultConfig()
	cfg.Version = 2
	cfg.KeepAliveInterval = 10 * time.Second
	cfg.KeepAliveTimeout = 30 * time.Second
	cfg.MaxFrameSize = 65535
	cfg.MaxReceiveBuffer = opt.SmuxBuf
	cfg.MaxStreamBuffer = opt.StreamBuf
	return cfg
}

func smuxClient(conn io.ReadWriteCloser, opt Options) (*smux.Session, error) {
	sess, err := smux.Client(conn, smuxConfig(opt))
	if err != nil {
		return nil, fmt.Errorf("smux client: %w", err)
	}
	return sess, nil
}

func smuxServer(conn io.ReadWriteCloser, opt Options) (*smux.Session, error) {
	sess, err := smux.Server(conn, smuxConfig(opt))
	if err != nil {
		return nil, fmt.Errorf("smux server: %w", err)
	}
	return sess, nil
}

func Dial(ctx context.Context, addr *net.UDPAddr, opt Options, pconn *pcap.Conn) (*Session, error) {
	switch opt.Protocol {
	case ProtocolQUIC:
		return dialQUIC(ctx, addr, opt, pconn)
	default:
		return dialKCP(addr, opt, pconn)
	}
}

func dialKCP(addr *net.UDPAddr, opt Options, pconn *pcap.Conn) (*Session, error) {
	kcpConn, err := kcp.NewConn(addr.String(), opt.KCP.Block, 0, 0, pconn)
	if err != nil {
		return nil, fmt.Errorf("kcp dial: %w", err)
	}
	ApplyKCP(kcpConn, opt)

	sess, err := smuxClient(kcpConn, opt)
	if err != nil {
		kcpConn.Close()
		return nil, err
	}
	return &Session{PacketConn: pconn, KCP: kcpConn, remoteAddr: addr, Smux: sess}, nil
}

type Listener struct {
	packetConn *pcap.Conn
	opt        Options
	kcpLn      *kcp.Listener
	quicLn     *quicListener
}

func Listen(opt Options, pconn *pcap.Conn) (*Listener, error) {
	switch opt.Protocol {
	case ProtocolQUIC:
		qln, err := listenQUIC(opt, pconn)
		if err != nil {
			return nil, err
		}
		return &Listener{packetConn: pconn, opt: opt, quicLn: qln}, nil
	default:
		ln, err := kcp.ServeConn(opt.KCP.Block, 0, 0, pconn)
		if err != nil {
			return nil, fmt.Errorf("kcp listen: %w", err)
		}
		return &Listener{packetConn: pconn, opt: opt, kcpLn: ln}, nil
	}
}

func (l *Listener) Accept(ctx context.Context) (*Session, error) {
	if l.quicLn != nil {
		return l.quicLn.Accept(ctx)
	}

	kcpConn, err := l.kcpLn.AcceptKCP()
	if err != nil {
		return nil, err
	}
	ApplyKCP(kcpConn, l.opt)

	sess, err := smuxServer(kcpConn, l.opt)
	if err != nil {
		kcpConn.Close()
		return nil, err
	}
	return &Session{KCP: kcpConn, remoteAddr: kcpConn.RemoteAddr(), Smux: sess}, nil
}

func (l *Listener) Close() error {
	if l.quicLn != nil {
		return l.quicLn.Close()
	}
	if l.kcpLn != nil {
		l.kcpLn.Close()
	}
	if l.packetConn != nil {
		l.packetConn.Close()
	}
	return nil
}

func (l *Listener) Addr() net.Addr {
	if l.kcpLn != nil {
		return l.kcpLn.Addr()
	}
	return nil
}
