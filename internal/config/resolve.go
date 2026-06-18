package config

import (
	"fmt"
	"math/rand"
	"net"
	"strings"

	"github.com/paqetpremium/paqetpremium/internal/netutil"
)

const (
	defaultPCAPClientBuf = 4 * 1024 * 1024
	defaultPCAPServerBuf = 8 * 1024 * 1024
)

type NetworkRuntime struct {
	Interface     *net.Interface
	IPv4          *net.UDPAddr
	RouterMAC     net.HardwareAddr
	IPv6          *net.UDPAddr
	IPv6RouterMAC net.HardwareAddr
	Port          int
	LocalFlags    []netutil.TCPFlagSet
	RemoteFlags   []netutil.TCPFlagSet
	Sockbuf       int
}

func (c *Config) ResolveNetwork() (*NetworkRuntime, error) {
	iface, err := net.InterfaceByName(c.Network.Interface)
	if err != nil {
		return nil, fmt.Errorf("network interface %q: %w", c.Network.Interface, err)
	}

	allowZero := c.Role == RoleClient
	ipv4, err := netutil.ParseUDPAddr(c.Network.IPv4.Addr, allowZero)
	if err != nil {
		return nil, fmt.Errorf("network.ipv4.addr: %w", err)
	}
	if ipv4.Port == 0 {
		ipv4.Port = 32768 + rand.Intn(32768)
	}

	mac, err := net.ParseMAC(c.Network.IPv4.RouterMAC)
	if err != nil {
		return nil, fmt.Errorf("network.ipv4.router_mac: %w", err)
	}

	localFlags, err := netutil.ParseTCPFlags(c.Network.TCP.LocalFlag, "PA")
	if err != nil {
		return nil, fmt.Errorf("network.tcp.local_flag: %w", err)
	}
	remoteFlags, err := netutil.ParseTCPFlags(c.Network.TCP.RemoteFlag, "PA")
	if err != nil {
		return nil, fmt.Errorf("network.tcp.remote_flag: %w", err)
	}

	sockbuf := defaultPCAPClientBuf
	if c.Role == RoleServer {
		sockbuf = defaultPCAPServerBuf
	}

	rt := &NetworkRuntime{
		Interface:   iface,
		IPv4:        ipv4,
		RouterMAC:   mac,
		Port:        ipv4.Port,
		LocalFlags:  localFlags,
		RemoteFlags: remoteFlags,
		Sockbuf:     sockbuf,
	}

	if c.Network.IPv6 != nil && strings.TrimSpace(c.Network.IPv6.Addr) != "" {
		ipv6, err := netutil.ParseUDPAddr(c.Network.IPv6.Addr, allowZero)
		if err != nil {
			return nil, fmt.Errorf("network.ipv6.addr: %w", err)
		}
		if ipv6.Port == 0 {
			ipv6.Port = ipv4.Port
		}
		v6mac, err := net.ParseMAC(c.Network.IPv6.RouterMAC)
		if err != nil {
			return nil, fmt.Errorf("network.ipv6.router_mac: %w", err)
		}
		rt.IPv6 = ipv6
		rt.IPv6RouterMAC = v6mac
	}

	return rt, nil
}

// ResolveNetworkWithPort resolves network settings using an explicit local port.
// port 0 picks a random high port (client mode).
func (c *Config) ResolveNetworkWithPort(port int) (*NetworkRuntime, error) {
	addr := c.Network.IPv4.Addr
	if port > 0 {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("network.ipv4.addr: %w", err)
		}
		addr = fmt.Sprintf("%s:%d", host, port)
	}

	clone := *c
	clone.Network.IPv4.Addr = addr
	return clone.ResolveNetwork()
}

func (c *Config) RemoteServerAddr() (*net.UDPAddr, error) {
	if c.Server != nil && strings.TrimSpace(c.Server.Addr) != "" {
		return netutil.ParseUDPAddr(c.Server.Addr, false)
	}
	if c.Upstream != nil && len(c.Upstream.Servers) > 0 {
		return netutil.ParseUDPAddr(c.Upstream.Servers[0].Addr, false)
	}
	return nil, fmt.Errorf("no remote server configured")
}

func (c *Config) ListenUDPAddr() (*net.UDPAddr, error) {
	if c.Listen == nil {
		return nil, fmt.Errorf("listen.addr missing")
	}
	addr := strings.TrimSpace(c.Listen.Addr)
	if strings.HasPrefix(addr, ":") {
		addr = "0.0.0.0" + addr
	}
	return netutil.ParseUDPAddr(addr, false)
}
