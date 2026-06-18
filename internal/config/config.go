package config

import (
	"fmt"
	"os"
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
		if len(c.Forward) == 0 && len(c.SOCKS5) == 0 {
			return fmt.Errorf("client requires forward or socks5 rules")
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
