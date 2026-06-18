//go:build !linux

package pcap

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/config"
	"github.com/paqetpremium/paqetpremium/internal/netutil"
)

type Conn struct{}

func Open(ctx context.Context, netCfg *config.NetworkRuntime) (*Conn, error) {
	return nil, fmt.Errorf("pcap engine is only available on Linux")
}

func (c *Conn) ReadFrom(b []byte) (int, net.Addr, error)  { return 0, nil, fmt.Errorf("linux only") }
func (c *Conn) WriteTo(b []byte, addr net.Addr) (int, error) { return 0, fmt.Errorf("linux only") }
func (c *Conn) Close() error                               { return nil }
func (c *Conn) LocalAddr() net.Addr                        { return nil }
func (c *Conn) SetDeadline(t time.Time) error              { return nil }
func (c *Conn) SetReadDeadline(t time.Time) error          { return nil }
func (c *Conn) SetWriteDeadline(t time.Time) error         { return nil }

func (c *Conn) SetRemoteTCPFlags(addr net.Addr, flags []netutil.TCPFlagSet) {}
func (c *Conn) ClearRemoteTCPFlags(addr net.Addr)                         {}

func Probe(netCfg *config.NetworkRuntime) error {
	return fmt.Errorf("pcap probe requires Linux")
}

// Ensure stub satisfies net.PacketConn at compile time.
var _ net.PacketConn = (*Conn)(nil)

// Silence unused import on non-linux toolchains.
var _ = os.ErrInvalid
