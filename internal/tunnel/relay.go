package tunnel

import (
	"context"
	"net"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/ioext"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/xtaci/smux"
)

func relayTCP(ctx context.Context, strm *smux.Stream, addr string) error {
	metrics.Default.RelayTCP.Add(1)
	metrics.Default.TCPActive.Add(1)
	defer metrics.Default.TCPActive.Add(-1)

	dialer := net.Dialer{Timeout: 10 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		metrics.Default.IncError()
		return err
	}
	defer conn.Close()

	errCh := make(chan error, 2)
	go func() { errCh <- ioext.CopyMetered(conn, strm, metrics.Default, false) }()
	go func() { errCh <- ioext.CopyMetered(strm, conn, metrics.Default, true) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

func relayUDP(ctx context.Context, strm *smux.Stream, addr string) error {
	metrics.Default.RelayUDP.Add(1)

	dialer := net.Dialer{Timeout: 8 * time.Second}
	conn, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		metrics.Default.IncError()
		return err
	}
	defer conn.Close()

	errCh := make(chan error, 2)
	go func() { errCh <- ioext.CopyMetered(conn, strm, metrics.Default, false) }()
	go func() { errCh <- ioext.CopyMetered(strm, conn, metrics.Default, true) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
