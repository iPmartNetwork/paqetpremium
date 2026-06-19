package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
)

const (
	RoleClient = "client"
	RoleServer = "server"
)

type Config struct {
	Core    string `yaml:"core"`
	Version int    `yaml:"version"`
	Role    string `yaml:"role"`
	Name    string `yaml:"name"`

	Log LogConfig `yaml:"log"`

	Upstream *UpstreamConfig `yaml:"upstream,omitempty"`

	Forward []ForwardRule `yaml:"forward,omitempty"`
	SOCKS5  []SOCKS5Rule  `yaml:"socks5,omitempty"`

	Range *RangeConfig `yaml:"range,omitempty"`

	Listen *ListenConfig `yaml:"listen,omitempty"`

	Network   NetworkConfig   `yaml:"network"`
	Server    *ServerEndpoint `yaml:"server,omitempty"`
	Transport TransportConfig `yaml:"transport"`

	Admin *AdminConfig `yaml:"admin,omitempty"`
}

type LogConfig struct {
	Level string `yaml:"level"`
}

type UpstreamConfig struct {
	Strategy    string              `yaml:"strategy"`
	HealthCheck HealthCheckConfig   `yaml:"health_check"`
	Servers     []UpstreamServer    `yaml:"servers"`
}

type HealthCheckConfig struct {
	Interval         string `yaml:"interval"`
	Timeout          string `yaml:"timeout"`
	FailThreshold    int    `yaml:"fail_threshold"`
	RecoverThreshold int    `yaml:"recover_threshold"`
}

type UpstreamServer struct {
	Name     string `yaml:"name"`
	Addr     string `yaml:"addr"`
	Key      string `yaml:"key"`
	Weight   int    `yaml:"weight"`
	Priority int    `yaml:"priority"`
}

type ForwardRule struct {
	Listen        string `yaml:"listen"`
	Target        string `yaml:"target"`
	Protocol      string `yaml:"protocol"`
	BindUpstream  string `yaml:"bind_upstream,omitempty"`
}

type SOCKS5Rule struct {
	Listen string     `yaml:"listen"`
	Auth   *SOCKSAuth `yaml:"auth,omitempty"`
}

type SOCKSAuth struct {
	User string `yaml:"user"`
	Pass string `yaml:"pass"`
}

// RangeConfig enables transparent "tunnel all ports" mode on the client: a
// single local listener plus an iptables nat REDIRECT for a port range. Each
// redirected connection is tunneled to target_host:<original destination port>.
type RangeConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Protocol     string `yaml:"protocol"`      // tcp (only tcp supported)
	RedirectPort int    `yaml:"redirect_port"` // local REDIRECT listener port
	TargetHost   string `yaml:"target_host"`   // server-side dial host (e.g. 127.0.0.1)
	Ports        string `yaml:"ports"`         // e.g. "1-65535" or "443,8443,2000-3000"
	Exclude      string `yaml:"exclude"`       // ports never redirected, e.g. "22,9090"
	BindUpstream string `yaml:"bind_upstream,omitempty"`
}

// PortRanges parses Ports into inclusive [lo,hi] ranges.
func (r *RangeConfig) PortRanges() ([][2]int, error) { return parsePortRanges(r.Ports) }

// ExcludePorts parses Exclude into a list of single ports.
func (r *RangeConfig) ExcludePorts() ([]int, error) {
	var out []int
	for _, part := range strings.Split(r.Exclude, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		p, err := strconv.Atoi(part)
		if err != nil || p < 1 || p > 65535 {
			return nil, fmt.Errorf("invalid exclude port %q", part)
		}
		out = append(out, p)
	}
	return out, nil
}

func parsePortRanges(s string) ([][2]int, error) {
	var out [][2]int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			lohi := strings.SplitN(part, "-", 2)
			lo, err1 := strconv.Atoi(strings.TrimSpace(lohi[0]))
			hi, err2 := strconv.Atoi(strings.TrimSpace(lohi[1]))
			if err1 != nil || err2 != nil || lo < 1 || hi > 65535 || lo > hi {
				return nil, fmt.Errorf("invalid port range %q", part)
			}
			out = append(out, [2]int{lo, hi})
		} else {
			p, err := strconv.Atoi(part)
			if err != nil || p < 1 || p > 65535 {
				return nil, fmt.Errorf("invalid port %q", part)
			}
			out = append(out, [2]int{p, p})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no ports specified")
	}
	return out, nil
}

type ListenConfig struct {
	Addr string `yaml:"addr"`
}

type NetworkConfig struct {
	Interface string    `yaml:"interface"`
	IPv4      IPv4Config `yaml:"ipv4"`
	IPv6      *IPv6Config `yaml:"ipv6,omitempty"`
	TCP       TCPFlags  `yaml:"tcp"`
}

type IPv4Config struct {
	Addr      string `yaml:"addr"`
	RouterMAC string `yaml:"router_mac"`
}

type IPv6Config struct {
	Addr      string `yaml:"addr"`
	RouterMAC string `yaml:"router_mac"`
}

type TCPFlags struct {
	LocalFlag  []string `yaml:"local_flag"`
	RemoteFlag []string `yaml:"remote_flag"`
}

type ServerEndpoint struct {
	Addr string `yaml:"addr"`
}

type TransportConfig struct {
	Protocol string      `yaml:"protocol"`
	Conn     int         `yaml:"conn"`
	KCP      KCPConfig   `yaml:"kcp"`
	QUIC     QUICConfig  `yaml:"quic,omitempty"`
}

type KCPConfig struct {
	Mode  string `yaml:"mode"`
	Block string `yaml:"block"`
	Key   string `yaml:"key"`
	MTU   int    `yaml:"mtu"`
}

type QUICConfig struct {
	ALPN           string `yaml:"alpn"`
	IdleTimeout    string `yaml:"idle_timeout"`
	MaxIdleTimeout string `yaml:"max_idle_timeout"`
}

type AdminConfig struct {
	Listen  string `yaml:"listen"`
	Metrics bool   `yaml:"metrics"`
	Token   string `yaml:"token,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	role := strings.ToLower(strings.TrimSpace(c.Role))
	switch role {
	case RoleClient, RoleServer:
		c.Role = role
	default:
		return fmt.Errorf("role must be %q or %q", RoleClient, RoleServer)
	}

	if strings.TrimSpace(c.Network.Interface) == "" {
		return fmt.Errorf("network.interface is required")
	}
	if strings.TrimSpace(c.Network.IPv4.Addr) == "" {
		return fmt.Errorf("network.ipv4.addr is required")
	}
	if strings.TrimSpace(c.Network.IPv4.RouterMAC) == "" {
		return fmt.Errorf("network.ipv4.router_mac is required")
	}
	if c.Network.IPv6 != nil && strings.TrimSpace(c.Network.IPv6.Addr) != "" {
		if strings.TrimSpace(c.Network.IPv6.RouterMAC) == "" {
			return fmt.Errorf("network.ipv6.router_mac is required when ipv6.addr is set")
		}
	}

	proto := strings.ToLower(strings.TrimSpace(c.Transport.Protocol))
	if proto == "" {
		proto = "kcp"
		c.Transport.Protocol = proto
	}
	if proto != "kcp" && proto != "quic" {
		return fmt.Errorf("transport.protocol: must be kcp or quic")
	}
	if c.Transport.Conn < 1 {
		c.Transport.Conn = 6
	}
	if strings.TrimSpace(c.Transport.KCP.Key) == "" {
		return fmt.Errorf("transport.kcp.key is required (shared secret for kcp and quic)")
	}
	if proto == "kcp" {
		if strings.TrimSpace(c.Transport.KCP.Mode) == "" {
			c.Transport.KCP.Mode = "fast"
		}
		if strings.TrimSpace(c.Transport.KCP.Block) == "" {
			c.Transport.KCP.Block = "aes-128-gcm"
		}
		if c.Transport.KCP.MTU <= 0 {
			c.Transport.KCP.MTU = 1150
		}
	}
	if proto == "quic" {
		if strings.TrimSpace(c.Transport.QUIC.ALPN) == "" {
			c.Transport.QUIC.ALPN = "paqetpremium"
		}
	}

	switch c.Role {
	case RoleServer:
		if c.Listen == nil || strings.TrimSpace(c.Listen.Addr) == "" {
			return fmt.Errorf("listen.addr is required for server role")
		}
	case RoleClient:
		if c.Upstream == nil || len(c.Upstream.Servers) == 0 {
			if c.Server == nil || strings.TrimSpace(c.Server.Addr) == "" {
				return fmt.Errorf("client requires upstream.servers or server.addr")
			}
		}
		if c.Range != nil && c.Range.Enabled {
			if strings.TrimSpace(c.Range.Protocol) == "" {
				c.Range.Protocol = "tcp"
			}
			if c.Range.Protocol != "tcp" {
				return fmt.Errorf("range.protocol: only tcp is supported")
			}
			if c.Range.RedirectPort <= 0 {
				c.Range.RedirectPort = 47999
			}
			if strings.TrimSpace(c.Range.TargetHost) == "" {
				c.Range.TargetHost = "127.0.0.1"
			}
			if strings.TrimSpace(c.Range.Ports) == "" {
				c.Range.Ports = "1-65535"
			}
			if strings.TrimSpace(c.Range.Exclude) == "" {
				c.Range.Exclude = "22"
			}
			if _, err := c.Range.PortRanges(); err != nil {
				return fmt.Errorf("range.ports: %w", err)
			}
			if _, err := c.Range.ExcludePorts(); err != nil {
				return fmt.Errorf("range.exclude: %w", err)
			}
		}
		if len(c.Forward) == 0 && len(c.SOCKS5) == 0 && (c.Range == nil || !c.Range.Enabled) {
			return fmt.Errorf("client requires forward, socks5, or range rules")
		}
	}

	if c.Upstream != nil {
		if strings.TrimSpace(c.Upstream.Strategy) == "" {
			c.Upstream.Strategy = "failover"
		}
		for i, s := range c.Upstream.Servers {
			if strings.TrimSpace(s.Addr) == "" {
				return fmt.Errorf("upstream.servers[%d].addr is required", i)
			}
			if strings.TrimSpace(s.Key) == "" {
				return fmt.Errorf("upstream.servers[%d].key is required", i)
			}
		}
	}

	if strings.TrimSpace(c.Log.Level) == "" {
		c.Log.Level = "info"
	}

	return nil
}
