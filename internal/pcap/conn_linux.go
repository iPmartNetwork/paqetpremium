//go:build linux

package pcap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gopacket/gopacket/pcap"
	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/netutil"
)

var logReadErr sync.Once

// Conn implements net.PacketConn over crafted TCP packets on a Linux interface.
type Conn struct {
	net        *config.NetworkRuntime
	send       *sendHandle
	recv       *recvHandle
	readDeadline  atomic.Value
	writeDeadline atomic.Value
	ctx        context.Context
	cancel     context.CancelFunc
}

func Open(ctx context.Context, netCfg *config.NetworkRuntime) (*Conn, error) {
	send, err := newSendHandle(netCfg)
	if err != nil {
		return nil, err
	}
	recv, err := newRecvHandle(netCfg)
	if err != nil {
		send.close()
		return nil, err
	}

	ctx, cancel := context.WithCancel(ctx)
	return &Conn{
		net:    netCfg,
		send:   send,
		recv:   recv,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (c *Conn) ReadFrom(buf []byte) (int, net.Addr, error) {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := c.readDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	for {
		select {
		case <-c.ctx.Done():
			return 0, nil, c.ctx.Err()
		case <-deadline:
			return 0, nil, os.ErrDeadlineExceeded
		default:
		}

		payload, addr, err := c.recv.read()
		if err != nil {
			if errors.Is(err, pcap.NextErrorTimeoutExpired) {
				continue
			}
			if errors.Is(err, io.EOF) {
				// Some libpcap builds in non-blocking/immediate mode report an
				// idle poll (no packet ready) as io.EOF rather than a timeout.
				// Treat it as benign and keep polling; the brief sleep prevents a
				// busy loop. Real shutdown is handled by the ctx.Done() check at
				// the top of the loop (Conn.Close cancels ctx before closing the
				// handle), so this cannot spin forever on a closed conn.
				time.Sleep(time.Millisecond)
				continue
			}
			logReadErr.Do(func() {
				slog.Default().Error("pcap read error (closing conn)", "err", err)
			})
			return 0, nil, err
		}
		if payload == nil {
			continue
		}
		return copy(buf, payload), addr, nil
	}
}

func (c *Conn) WriteTo(buf []byte, addr net.Addr) (int, error) {
	var timer *time.Timer
	var deadline <-chan time.Time
	if d, ok := c.writeDeadline.Load().(time.Time); ok && !d.IsZero() {
		timer = time.NewTimer(time.Until(d))
		defer timer.Stop()
		deadline = timer.C
	}

	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	case <-deadline:
		return 0, os.ErrDeadlineExceeded
	default:
	}

	ua, ok := addr.(*net.UDPAddr)
	if !ok {
		return 0, net.InvalidAddrError("expected UDPAddr")
	}
	if err := c.send.write(buf, ua); err != nil {
		return 0, err
	}
	return len(buf), nil
}

func (c *Conn) Close() error {
	c.cancel()
	if c.send != nil {
		c.send.close()
	}
	if c.recv != nil {
		c.recv.close()
	}
	return nil
}

func (c *Conn) LocalAddr() net.Addr { return nil }

func (c *Conn) SetDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	c.writeDeadline.Store(t)
	return nil
}

func (c *Conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Store(t)
	return nil
}

func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline.Store(t)
	return nil
}

func (c *Conn) SetRemoteTCPFlags(addr net.Addr, flags []netutil.TCPFlagSet) {
	c.send.setRemoteFlags(addr, flags)
}

func (c *Conn) ClearRemoteTCPFlags(addr net.Addr) {
	c.send.deleteRemoteFlags(addr)
}

// Probe opens and immediately closes a pcap session to validate interface access.
func Probe(netCfg *config.NetworkRuntime) error {
	c, err := Open(context.Background(), netCfg)
	if err != nil {
		return err
	}
	return c.Close()
}

var _ net.PacketConn = (*Conn)(nil)
