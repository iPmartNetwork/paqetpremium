package tunnel

import (
	"context"
	"net"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/ioext"
	"github.com/paqetpremium/paqetpremium/internal/metrics"
	"github.com/paqetpremium/paqetpremium/internal/protocol"
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
	// stream -> UDP target (decode length-prefixed datagrams)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, err := protocol.ReadDatagram(strm, buf)
			if err != nil {
				errCh <- err
				return
			}
			if n == 0 {
				continue
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				errCh <- err
				return
			}
			metrics.Default.BytesOut.Add(uint64(n))
		}
	}()
	// UDP target -> stream (frame each datagram)
	go func() {
		buf := make([]byte, 64*1024)
		for {
			_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
			n, err := conn.Read(buf)
			if err != nil {
				errCh <- err
				return
			}
			if err := protocol.WriteDatagram(strm, buf[:n]); err != nil {
				errCh <- err
				return
			}
			metrics.Default.BytesIn.Add(uint64(n))
		}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
