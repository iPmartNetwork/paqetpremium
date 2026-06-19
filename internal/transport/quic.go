package transport

import (
	"context"
	"fmt"
	"net"

	"github.com/quic-go/quic-go"
)

func dialQUIC(ctx context.Context, addr *net.UDPAddr, opt Options, pconn net.PacketConn) (*Session, error) {
	tlsConf, err := tlsConfigFromSecret(opt.SecretKey, opt.QUIC.ALPN, false)
	if err != nil {
		return nil, fmt.Errorf("quic tls: %w", err)
	}

	qconf := &quic.Config{
		MaxIdleTimeout:  opt.QUIC.MaxIdleTimeout,
		KeepAlivePeriod: opt.QUIC.IdleTimeout,
	}

	tr := &quic.Transport{Conn: pconn}
	qconn, err := tr.Dial(ctx, addr, tlsConf, qconf)
	if err != nil {
		tr.Close()
		return nil, fmt.Errorf("quic dial: %w", err)
	}

	stream, err := qconn.OpenStreamSync(ctx)
	if err != nil {
		qconn.CloseWithError(0, "smux")
		tr.Close()
		return nil, fmt.Errorf("quic stream: %w", err)
	}

	smuxSess, err := smuxClient(stream, opt)
	if err != nil {
		stream.Close()
		qconn.CloseWithError(0, "smux")
		tr.Close()
		return nil, err
	}

	return &Session{
		PacketConn: pconn,
		remoteAddr: addr,
		quicTr:     tr,
		quicConn:   qconn,
		quicStream: stream,
		Smux:       smuxSess,
	}, nil
}

type quicListener struct {
	packetConn net.PacketConn
	opt        Options
	transport  *quic.Transport
	listener   *quic.Listener
}

func listenQUIC(opt Options, pconn net.PacketConn) (*quicListener, error) {
	tlsConf, err := tlsConfigFromSecret(opt.SecretKey, opt.QUIC.ALPN, true)
	if err != nil {
		return nil, fmt.Errorf("quic tls: %w", err)
	}

	qconf := &quic.Config{
		MaxIdleTimeout:  opt.QUIC.MaxIdleTimeout,
		KeepAlivePeriod: opt.QUIC.IdleTimeout,
	}

	tr := &quic.Transport{Conn: pconn}
	ln, err := tr.Listen(tlsConf, qconf)
	if err != nil {
		tr.Close()
		return nil, fmt.Errorf("quic listen: %w", err)
	}

	return &quicListener{
		packetConn: pconn,
		opt:        opt,
		transport:  tr,
		listener:   ln,
	}, nil
}

func (l *quicListener) Accept(ctx context.Context) (*Session, error) {
	qconn, err := l.listener.Accept(ctx)
	if err != nil {
		return nil, err
	}

	stream, err := qconn.AcceptStream(ctx)
	if err != nil {
		qconn.CloseWithError(0, "smux")
		return nil, fmt.Errorf("quic accept stream: %w", err)
	}

	smuxSess, err := smuxServer(stream, l.opt)
	if err != nil {
		stream.Close()
		qconn.CloseWithError(0, "smux")
		return nil, err
	}

	return &Session{
		remoteAddr: qconn.RemoteAddr(),
		quicConn:   qconn,
		quicStream: stream,
		Smux:       smuxSess,
	}, nil
}

func (l *quicListener) Close() error {
	if l.listener != nil {
		_ = l.listener.Close()
	}
	if l.transport != nil {
		_ = l.transport.Close()
	}
	if l.packetConn != nil {
		_ = l.packetConn.Close()
	}
	return nil
}
